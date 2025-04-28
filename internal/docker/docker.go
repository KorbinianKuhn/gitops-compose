package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

type Docker struct {
	url string
	username string
	password string
}

func NewDocker(url, username, password string) *Docker {
	return &Docker{
		url: url,
		username: username,
		password: password,
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

func (d Docker) LoginIfCredentialsSet() (bool, error) {
	if d.url == "" {
		return false, nil
	}

	cli, err := d.getClient()
	if err != nil {
		return false, err
	}
	defer cli.Close()
	
	authConfig := registry.AuthConfig{
		Username: d.username,
		Password: d.password,
		ServerAddress: d.url,
	}

	_, err = cli.RegistryLogin(context.Background(), authConfig)
	if err != nil {
		return false, fmt.Errorf("docker login failed: %w", err)
	}

	return true, nil
}