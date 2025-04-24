package docker

import (
	"fmt"
	"os/exec"
)

var (
	ErrLoginFailed = fmt.Errorf("docker login failed")
	ErrLogoutFailed = fmt.Errorf("docker logout failed")
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

func (Docker) VerifySocketConnection() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker socket connection failed: %w", err)
	}
	return nil
}

func (d Docker) AreCredentialsSet() bool {
	if d.url != "" && d.username != "" && d.password != "" {
		return true
	}
	return false
}

func (d Docker) login() error {
	cmd := exec.Command("docker", "login", d.url, "-u", d.username, "-p", d.password)
	if err := cmd.Run(); err != nil {
		return ErrLoginFailed
	}
	return nil
}

func (Docker) logout() error {
	cmd := exec.Command("docker", "logout")
	if err := cmd.Run(); err != nil {
		return ErrLogoutFailed
	}
	return nil
}

func (d Docker) VerifyCredentialsIfSet() error {
	if !d.AreCredentialsSet() {
		return nil
	}
	if err := d.login(); err != nil {
		return err
	}
	if err := d.logout(); err != nil {
		return err
	}
	return nil
}

func (d Docker) LoginIfCredentialsSet() error {
	if !d.AreCredentialsSet() {
		return nil
	}
	if err := d.login(); err != nil {
		return err
	}
	return nil
}

func (d Docker) LogoutIfCredentialsSet() error {
	if !d.AreCredentialsSet() {
		return nil
	}
	if err := d.logout(); err != nil {
		return err
	}
	return nil
}
