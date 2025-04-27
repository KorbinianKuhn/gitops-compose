package docker

import (
	"fmt"
	"os/exec"
)

type Docker struct {
	url string
	username string
	password string
	isLoggedIn bool
}

func NewDocker(url, username, password string) *Docker {
	return &Docker{
		url: url,
		username: username,
		password: password,
		isLoggedIn: false,
	}
}

func (Docker) VerifySocketConnection() error {
	cmd := exec.Command("docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker socket connection failed: %w %s", err, output)
	}
	return nil
}

func (d Docker) AreCredentialsSet() bool {
	if d.url != "" && d.username != "" && d.password != "" {
		return true
	}
	return false
}

func (d *Docker) login() error {
	if d.isLoggedIn {
		return nil
	}
	cmd := exec.Command("docker", "login", d.url, "-u", d.username, "-p", d.password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker login failed: %w %s", err, output)
	}
	d.isLoggedIn = true
	return nil
}

func (d *Docker) logout() error {
	if !d.isLoggedIn {
		return nil
	}
	cmd := exec.Command("docker", "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker logout failed: %w %s", err, output)
	}
	d.isLoggedIn = false
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
