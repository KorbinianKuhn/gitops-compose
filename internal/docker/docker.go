package docker

import (
	"context"
	"fmt"
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

func (d Docker) Pull(imageName string) error {
	cli, err := d.getClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	// TODO: eventually add auth to image.PullOptions
	reader, err := cli.ImagePull(context.Background(), imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker pull failed: %w", err)
	}
	defer reader.Close()

	return nil
}
