package compose

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"gopkg.in/yaml.v3"
)

type ComposeFile struct {
	Filepath string
}

func NewComposeFile(filepath string) *ComposeFile {
	return &ComposeFile{
		Filepath: filepath,
	}
}

func (c ComposeFile) LoadProject() (*types.Project, error) {
	ctx := context.Background()

	workingDirectory := path.Dir(c.Filepath)

	optionsFns := []cli.ProjectOptionsFn{}

	envFilePath := filepath.Join(workingDirectory, ".env")
	if _, err := os.Stat(envFilePath); err == nil {
		optionsFns = append(optionsFns,
			cli.WithEnvFiles(envFilePath),
			cli.WithDotEnv,
		)
	}

	optionsFns = append(optionsFns,
		cli.WithInterpolation(true),
		cli.WithWorkingDirectory(workingDirectory),
		cli.WithResolvedPaths(true),
	)

	options, err := cli.NewProjectOptions(
		[]string{c.Filepath},
		optionsFns...,
	)
	if err != nil {
		return &types.Project{}, fmt.Errorf("failed to create project options: %w", err)
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return &types.Project{}, fmt.Errorf("invalid compose file: %w", err)
	}

	return project, nil
}

type GitopsWatchConfig struct {
	Watch []string `yaml:"watch"`
}

func resolveWatchPath(projectDir, watchPath string) (string, error) {
	if filepath.IsAbs(watchPath) {
		return filepath.Clean(watchPath), nil
	}
	abs, err := filepath.Abs(filepath.Join(projectDir, watchPath))
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (c ComposeFile) GetWatchFiles(project *types.Project) []string {
	var watchFiles []string

	// Root-level x-gitops.watch
	if raw, ok := project.Extensions["x-gitops"]; ok {
		var cfg GitopsWatchConfig
		bytes, _ := yaml.Marshal(raw)
		if err := yaml.Unmarshal(bytes, &cfg); err != nil {
			slog.Warn("Failed to unmarshal x-gitops:", "compose", c.Filepath, "error", err)
		}
		watchFiles = append(watchFiles, cfg.Watch...)
	}

	// Service-level x-gitops.watch
	for _, service := range project.Services {
		if raw, ok := service.Extensions["x-gitops"]; ok {
			var cfg GitopsWatchConfig
			bytes, _ := yaml.Marshal(raw)
			if err := yaml.Unmarshal(bytes, &cfg); err != nil {
				slog.Warn("Failed to unmarshal x-gitops:", "compose", c.Filepath, "service", service.Name, "error", err)
			}
			for _, f := range cfg.Watch {
				if !slices.Contains(watchFiles, f) {
					watchFiles = append(watchFiles, f)
				}
			}
		}
	}

	var resolvedWatchFiles []string
	for _, f := range watchFiles {
		resolved, err := resolveWatchPath(project.WorkingDir, f)
		if err != nil {
			slog.Warn("Failed to resolve watch path:", "path", f, "error", err)
			continue
		}
		resolvedWatchFiles = append(resolvedWatchFiles, resolved)
	}

	return resolvedWatchFiles
}

func (c ComposeFile) ListImages() ([]string, error) {
	project, err := c.LoadProject()
	if err != nil {
		return []string{}, err
	}
	images := []string{}
	for _, service := range project.Services {
		images = append(images, service.Image)
	}

	return images, nil
}

func getService() (api.Service, error) {
	dockerCli, err := command.NewDockerCli(
		command.WithOutputStream(io.Discard),
		command.WithErrorStream(io.Discard),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker cli: %w", err)
	}

	opts := &flags.ClientOptions{Context: "default", LogLevel: "error"}

	if err := dockerCli.Initialize(opts); err != nil {
		return nil, fmt.Errorf("failed to initialize docker cli: %w", err)
	}

	return compose.NewComposeService(dockerCli), nil
}

func (c ComposeFile) IsRunning() (bool, error) {
	service, err := getService()
	if err != nil {
		return false, err
	}

	project, err := c.LoadProject()
	if err != nil {
		return false, err
	}

	ctx := context.Background()

	services := []string{}
	for _, s := range project.Services {
		services = append(services, s.Name)
	}

	containers, err := service.Ps(ctx, project.Name, api.PsOptions{
		Project:  project,
		All:      true,
		Services: services,
	})

	if err != nil {
		return false, fmt.Errorf("docker compose ps failed: %w", err)
	}

	if len(containers) == 0 {
		return false, nil
	}

	for _, container := range containers {
		if container.State == "running" {
			return true, nil
		}
	}

	return false, nil
}

func (c ComposeFile) Stop() error {
	service, err := getService()
	if err != nil {
		return err
	}

	project, err := c.LoadProject()
	if err != nil {
		return err
	}

	ctx := context.Background()

	if err := service.Down(ctx, project.Name, api.DownOptions{
		RemoveOrphans: true,
		Project:       project,
		Images:        "local",
		Volumes:       false,
	}); err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}

	return nil
}

func (c ComposeFile) Start() error {
	service, err := getService()
	if err != nil {
		return err
	}

	project, err := c.LoadProject()
	if err != nil {
		return err
	}

	for i, s := range project.Services {
		s.CustomLabels = map[string]string{
			api.ProjectLabel:     project.Name,
			api.ServiceLabel:     s.Name,
			api.VersionLabel:     api.ComposeVersion,
			api.WorkingDirLabel:  project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel:      "False", // default, will be overridden by `run` command
		}
		project.Services[i] = s
	}

	ctx := context.Background()

	err = service.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{
			RemoveOrphans:        true,
			Recreate:             api.RecreateForce,
			RecreateDependencies: api.RecreateForce,
			QuietPull:            true,
			AssumeYes:            true,
			Timeout:              func() *time.Duration { d := time.Duration(180) * time.Second; return &d }(),
		},
		Start: api.StartOptions{
			Project:     project,
			Wait:        true,
			WaitTimeout: time.Duration(180) * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	return nil
}
