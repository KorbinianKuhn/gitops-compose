package compose

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
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
	optionsFns := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(workingDirectory),
		cli.WithInterpolation(true),
		cli.WithResolvedPaths(true),
	}
	
	envFilePath := filepath.Join(workingDirectory, ".env")
	if _, err := os.Stat(envFilePath); err == nil {
		optionsFns = append(optionsFns, cli.WithEnvFiles(envFilePath), cli.WithDotEnv)
	}

	optionsFns = append(optionsFns, cli.WithInterpolation(true), cli.WithResolvedPaths(true))

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
	outputStream := io.Discard
	errorStream := io.Discard

	dockerCli, err := command.NewDockerCli(
		command.WithOutputStream(outputStream),
		command.WithErrorStream(errorStream),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker cli: %w", err)
	}

	opts := &flags.ClientOptions{Context: "default", LogLevel: "error"}

	err = dockerCli.Initialize(opts)
	if err != nil {
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
		Project: project,
		All: true,
		Services: services,
	})

	if err != nil {
		return false, fmt.Errorf("docker compose ps failed: %w", err)
	}

	if len(containers) == 0 {
		slog.Info("No containers found for project", "project", project.Name)
		return false, nil
	}

	for _, container := range containers {
		if container.State == "running" {
			return true, nil
		}
	}

	return false, nil
}

func (c ComposeFile) Stop() (error) {
	service, err := getService()
	if err != nil {
		return err
	}

	project, err := c.LoadProject()
	if err != nil {
		return err
	}

	ctx := context.Background()

	err = service.Down(ctx, project.Name, api.DownOptions{
		RemoveOrphans: true,
		Project: project,
	})

	if err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}

	return nil
}

func (c ComposeFile) Start() (error) {
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
			api.ProjectLabel: project.Name,
			api.ServiceLabel: s.Name,
			api.VersionLabel: api.ComposeVersion,
			api.WorkingDirLabel: project.WorkingDir,
			api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
			api.OneoffLabel: "False", // default, will be overridden by `run` command
		}
		project.Services[i] = s
	}

	ctx := context.Background()

	err = service.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{
			RemoveOrphans: true,
			Recreate: api.RecreateForce,
			RecreateDependencies: api.RecreateForce,
			QuietPull: true,
		},
		Start: api.StartOptions{
			Project: project,
			Wait: true,
			WaitTimeout: time.Duration(180) * time.Second,
		},
	})

	if err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	return nil
}