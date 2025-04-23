package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	RepositoryPath string `required:"true" split_words:"true"`
	RepositoryUsername string `required:"true" split_words:"true"`
	RepositoryPassword string `required:"true" split_words:"true"`
}


func Get() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}
	var config Config

	if err := envconfig.Process("", &config); err != nil {
		return nil, err
	}
	
	return &config, nil
}
