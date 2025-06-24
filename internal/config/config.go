package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"

	gogit "github.com/go-git/go-git/v5"
)

type LogFormatDecoder string

type LogLevelDecoder slog.Level
type Config struct {
	CheckIntervalInSeconds int                     `default:"300" split_words:"true"`
	RepositoryPath         string                  `required:"true" split_words:"true"`
	RepositoryUsername     string                  `ignored:"true"`
	RepositoryPassword     string                  `ignored:"true"`
	WebhookEnabled         bool                    `default:"true" split_words:"true"`
	MetricsEnabled         bool                    `default:"true" split_words:"true"`
	DockerRegistries       DockerRegistriesDecoder `default:"[]" split_words:"true"`
	IsRunningInDocker      bool                    `default:"false" split_words:"true"`
	LogFormat              LogFormatDecoder        `default:"text" split_words:"true"`
	LogLevel               LogLevelDecoder         `default:"info" split_words:"true"`
}

func getCredentialsFromRepository(path string) (string, string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ""
	}

	r, err := gogit.PlainOpen(path)
	if err != nil {
		return "", ""
	}

	origin, err := r.Remote("origin")
	if err != nil {
		return "", ""
	}

	var remoteURL string
	for _, u := range origin.Config().URLs {
		remoteURL = u
		break
	}

	if remoteURL == "" {
		return "", ""
	}

	u, err := url.Parse(remoteURL)
	if err != nil {
		return "", ""
	}

	var username, password string

	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	return username, password
}

type DockerRegistriesDecoder []docker.DockerRegistryCredentials

func (d *DockerRegistriesDecoder) Decode(value string) error {
	var registries []docker.DockerRegistryCredentials

	if err := json.Unmarshal([]byte(value), &registries); err != nil {
		return err
	}

	*d = DockerRegistriesDecoder(registries)

	return nil
}

func (f *LogFormatDecoder) UnmarshalText(text []byte) error {
	value := strings.ToLower(string(text))
	switch value {
	case "json", "text", "console":
		*f = LogFormatDecoder(value)
		return nil
	default:
		return fmt.Errorf("invalid log format: %s", value)
	}
}

func (l *LogLevelDecoder) UnmarshalText(text []byte) error {
	value := strings.ToLower(string(text))
	switch value {
	case "debug":
		*l = LogLevelDecoder(slog.LevelDebug)
	case "info":
		*l = LogLevelDecoder(slog.LevelInfo)
	case "warn":
		*l = LogLevelDecoder(slog.LevelWarn)
	case "error":
		*l = LogLevelDecoder(slog.LevelError)
	default:
		return fmt.Errorf("invalid log level: %s", value)
	}
	return nil
}

func Get() (*Config, error) {
	godotenv.Load()

	var config Config

	if err := envconfig.Process("", &config); err != nil {
		return nil, err
	}

	if config.IsRunningInDocker {
		if !filepath.IsAbs(config.RepositoryPath) {
			return nil, fmt.Errorf("repository path must be absolute when running in Docker, got: %s", config.RepositoryPath)
		}
	}

	// Get credentials from repository origin
	config.RepositoryUsername, config.RepositoryPassword = getCredentialsFromRepository(config.RepositoryPath)

	return &config, nil
}
