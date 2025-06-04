package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

type Docker struct {
	registries []DockerRegistryCredentials
}

type DockerRegistryCredentials struct {
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewDocker(registries []DockerRegistryCredentials) *Docker {
	return &Docker{
		registries: registries,
	}
}

func (d Docker) getClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return cli, nil
}

func (d Docker) VerifySocketConnection() error {
	cli, err := d.getClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	_, err = cli.Ping(context.Background())
	if err != nil {
		return fmt.Errorf("docker daemon is not reachable: %w", err)
	}

	return nil
}

func (d Docker) IsDockerDesktop() (bool, error) {
	cli, err := d.getClient()
	if err != nil {
		return false, err
	}
	defer cli.Close()

	info, err := cli.Info(context.Background())
	if err != nil {
		return false, fmt.Errorf("failed to get docker info: %w", err)
	}

	if strings.Contains(strings.ToLower(info.OperatingSystem), "docker desktop") {
		return true, nil
	}

	return false, nil
}

func (d Docker) LoginIfCredentialsSet() (bool, error) {
	if len(d.registries) == 0 {
		return false, nil
	}

	cli, err := d.getClient()
	if err != nil {
		return false, err
	}
	defer cli.Close()

	for _, r := range d.registries {
		authConfig := registry.AuthConfig{
			Username:      r.Username,
			Password:      r.Password,
			ServerAddress: r.Url,
		}

		_, err = cli.RegistryLogin(context.Background(), authConfig)
		if err != nil {
			return false, fmt.Errorf("docker login failed: %w", err)
		}
	}

	return true, nil
}

func filterRegistyCredentials(registries []DockerRegistryCredentials, imageName string) []DockerRegistryCredentials {
	matches := make([]DockerRegistryCredentials, 0)
	for _, r := range registries {
		if strings.HasPrefix(imageName, r.Url) {
			matches = append(matches, r)
		}
	}
	return matches
}

func tryPullWithOptions(cli *client.Client, imageName string, pullOptions image.PullOptions) error {
	reader, err := cli.ImagePull(context.Background(), imageName, pullOptions)
	if err != nil {
		return err
	}
	defer reader.Close()

	decoder := json.NewDecoder(reader)

	for {
		var msg map[string]any
		if err := decoder.Decode(&msg); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to decode docker pull response: %w", err)
		}
	}
	return nil
}

func ImageExistsLocally(cli *client.Client, image string) (bool, error) {
	_, err := cli.ImageInspect(context.TODO(), image)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d Docker) Pull(imageName string) error {
	cli, err := d.getClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	exists, err := ImageExistsLocally(cli, imageName)
	if err != nil {
		slog.Warn("failed to check if image exists locally", "image", imageName, "error", err)
	}

	// If the image already exists locally, no need to pull it again
	if exists {
		return nil
	}

	// Try pulling with registry credentials
	slog.Info("pulling image", "name", imageName)
	registries := filterRegistyCredentials(d.registries, imageName)
	for _, r := range registries {
		encodedAuthConfig, err := registry.EncodeAuthConfig(registry.AuthConfig{
			Username:      r.Username,
			Password:      r.Password,
			ServerAddress: r.Url,
		})

		if err != nil {
			slog.Warn("failed to encode registry auth config", "registry", r.Url, "error", err)
			continue
		}

		pullOptions := image.PullOptions{
			RegistryAuth: encodedAuthConfig,
		}

		err = tryPullWithOptions(cli, imageName, pullOptions)
		if err != nil {
			slog.Warn("failed to pull image with registry credentials", "registry", r.Url, "error", err)
			continue
		}

		return nil
	}

	// Try pulling without registry credentials
	err = tryPullWithOptions(cli, imageName, image.PullOptions{})
	if err != nil {
		slog.Warn("failed to pull image without registry credentials", "error", err)
	} else {
		return nil
	}

	return fmt.Errorf("failed to pull image %s: no valid registry credentials found", imageName)
}
