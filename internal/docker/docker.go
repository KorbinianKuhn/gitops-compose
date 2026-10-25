package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
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
	cli, err := client.New(client.FromEnv)

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

	_, err = cli.Ping(context.Background(), client.PingOptions{})
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

	result, err := cli.Info(context.Background(), client.InfoOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get docker info: %w", err)
	}

	if strings.Contains(strings.ToLower(result.Info.OperatingSystem), "docker desktop") {
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
		loginOptions := client.RegistryLoginOptions{
			Username:      r.Username,
			Password:      r.Password,
			ServerAddress: r.Url,
		}

		_, err = cli.RegistryLogin(context.Background(), loginOptions)
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

// Wait reports pull failures sent inside the response stream instead of the HTTP status
func tryPullWithOptions(cli *client.Client, imageName string, pullOptions client.ImagePullOptions) error {
	resp, err := cli.ImagePull(context.Background(), imageName, pullOptions)
	if err != nil {
		return err
	}

	return resp.Wait(context.Background())
}

func ImageExistsLocally(cli *client.Client, image string) (bool, error) {
	_, err := cli.ImageInspect(context.TODO(), image)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
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
		encodedAuthConfig, err := authconfig.Encode(registry.AuthConfig{
			Username:      r.Username,
			Password:      r.Password,
			ServerAddress: r.Url,
		})

		if err != nil {
			slog.Warn("failed to encode registry auth config", "registry", r.Url, "error", err)
			continue
		}

		pullOptions := client.ImagePullOptions{
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
	err = tryPullWithOptions(cli, imageName, client.ImagePullOptions{})
	if err != nil {
		slog.Warn("failed to pull image without registry credentials", "error", err)
	} else {
		return nil
	}

	return fmt.Errorf("failed to pull image %s: no valid registry credentials found", imageName)
}
