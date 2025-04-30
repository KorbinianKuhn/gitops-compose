package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)


type Config struct {
	CheckIntervalInSeconds int `default:"300" split_words:"true"`
	RepositoryPath string `default:"/repository" split_words:"true"`
	RepositoryUsername string `required:"true" split_words:"true"`
	RepositoryPassword string `required:"true" split_words:"true"`
	DockerRegistryUrl string `required:"false" split_words:"true"`
	DockerRegistryUsername string `required:"false" split_words:"true"`
	DockerRegistryPassword string `required:"false" split_words:"true"`
	DisableWebhook bool `default:"false" split_words:"true"`
	DisableMetrics bool `default:"false" split_words:"true"`
}


func Get() (*Config, error) {
	godotenv.Load()

	var config Config

	if err := envconfig.Process("", &config); err != nil {
		return nil, err
	}

	if config.DockerRegistryUrl != "" {
		if config.DockerRegistryUsername == "" || config.DockerRegistryPassword == "" {
			return nil, fmt.Errorf("docker registry username and password are required when docker registry url is set")
		}
	}
	
	return &config, nil
}
