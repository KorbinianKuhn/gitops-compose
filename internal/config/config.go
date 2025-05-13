package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	CheckIntervalInSeconds int    `default:"300" split_words:"true"`
	RepositoryPath         string `default:"/repository" split_words:"true"`
	RepositoryUsername     string `required:"false" split_words:"true"`
	RepositoryPassword     string `required:"false" split_words:"true"`
	DockerRegistryUrl      string `required:"false" split_words:"true"`
	DockerRegistryUsername string `required:"false" split_words:"true"`
	DockerRegistryPassword string `required:"false" split_words:"true"`
	DisableWebhook         bool   `default:"false" split_words:"true"`
	DisableMetrics         bool   `default:"false" split_words:"true"`
}

func Get() (*Config, error) {
	godotenv.Load()

	var config Config

	if err := envconfig.Process("", &config); err != nil {
		return nil, err
	}

	// Use the repository credentials for the docker registry if not set
	if config.DockerRegistryUrl != "" {
		if config.DockerRegistryUsername == "" && config.RepositoryUsername != "" {
			config.DockerRegistryUsername = config.RepositoryUsername
		}
		if config.DockerRegistryPassword == "" && config.RepositoryPassword != "" {
			config.DockerRegistryPassword = config.RepositoryPassword
		}
	}

	return &config, nil
}

func (c *Config) SetCredentials(username, password string) {
	if c.RepositoryUsername == "" && username != "" {
		c.RepositoryUsername = username
	}
	if c.RepositoryPassword == "" && password != "" {
		c.RepositoryPassword = password
	}

	if c.DockerRegistryUrl != "" {
		if c.DockerRegistryUsername == "" && c.RepositoryUsername != "" {
			c.DockerRegistryUsername = c.RepositoryUsername
		}
		if c.DockerRegistryPassword == "" && c.RepositoryPassword != "" {
			c.DockerRegistryPassword = c.RepositoryPassword
		}
	}
}
