package git

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

var (
	ErrPathDoesNotExist = fmt.Errorf("path does not exist")
	ErrHasLocalChanges  = fmt.Errorf("local changes detected")
)

type DeploymentRepo struct {
	auth *gitHttp.BasicAuth
	path string
}

type DeploymentRepoOption func(*DeploymentRepo)

func WithAuth(username, password string) DeploymentRepoOption {
	return func(r *DeploymentRepo) {
		r.auth = &gitHttp.BasicAuth{
			Username: username,
			Password: password,
		}
	}
}

func NewDeploymentRepo(path string, opts ...DeploymentRepoOption) (*DeploymentRepo, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ErrPathDoesNotExist
	}

	r, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open repo failed: %w", err)
	}

	origin, err := r.Remote("origin")
	if err != nil {
		return nil, fmt.Errorf("get remote failed: %w", err)
	}

	var remoteURL string
	for _, u := range origin.Config().URLs {
		remoteURL = u
		break
	}

	if remoteURL == "" {
		return nil, fmt.Errorf("remote url not found")
	}

	repo := &DeploymentRepo{
		path: path,
	}

	for _, opt := range opts {
		opt(repo)
	}

	return repo, nil
}

func (r DeploymentRepo) VerifyRemoteAccess() error {
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return fmt.Errorf("open repo failed: %w", err)
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("get remote failed: %w", err)
	}

	listOptions := &gogit.ListOptions{}
	if r.auth != nil {
		listOptions.Auth = r.auth
	}

	_, err = remote.List(listOptions)
	if err != nil {
		return fmt.Errorf("remote is not working or auth failed: %w", err)
	}

	return nil
}

func (r DeploymentRepo) HasChanges() (bool, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return false, fmt.Errorf("open repo failed: %w", err)
	}

	// Get the working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("get worktree failed: %w", err)
	}

	// Check if the working tree is clean
	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("get status failed: %w", err)
	}

	// If there are changes, we cannot savely proceed
	if !status.IsClean() {
		return false, ErrHasLocalChanges
	}

	// Fetch the latest changes from the remote repository
	err = repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
		Auth:       r.auth,
		Tags:       gogit.NoTags,
		Force:      false,
		Prune:      false,
	})
	if err != nil {
		if err == gogit.NoErrAlreadyUpToDate {
			return false, nil
		}
		return false, fmt.Errorf("fetch failed: %w", err)
	}

	// Get the local references for the main branch
	localRef, err := repo.Reference(plumbing.ReferenceName("refs/heads/main"), true)
	if err != nil {
		return false, fmt.Errorf("get local ref failed: %w", err)
	}

	// Get the remote references for the main branch
	remoteRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return false, fmt.Errorf("get remote ref failed: %w", err)
	}

	// Compare the hashes of the local and remote references
	if localRef.Hash() == remoteRef.Hash() {
		return false, nil
	} else {
		return true, nil
	}
}

func (r DeploymentRepo) filterComposeFiles(c object.Commit) ([]string, error) {
	// Get the tree of the commit
	tree, err := c.Tree()
	if err != nil {
		return nil, fmt.Errorf("get tree failed: %w", err)
	}

	// Iterate through the files in the tree
	var composeFiles []string
	err = tree.Files().ForEach(func(f *object.File) error {
		filename := path.Base(f.Name)
		if filename == "docker-compose.yml" {
			filepath := path.Join(r.path, f.Name)
			composeFiles = append(composeFiles, filepath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk tree failed: %w", err)
	}

	return composeFiles, nil
}

func (r DeploymentRepo) GetRemoteComposeFiles() ([]string, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return nil, fmt.Errorf("open repo failed: %w", err)
	}

	// Get the remote references for the main branch
	ref, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return nil, fmt.Errorf("get remote ref failed: %w", err)
	}

	// Get the latest commit from the remote main branch
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("get commit object failed: %w", err)
	}

	return r.filterComposeFiles(*commit)
}

func (r DeploymentRepo) GetLocalComposeFiles() ([]string, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return nil, fmt.Errorf("open repo failed: %w", err)
	}

	// Get the local references for the main branch
	ref, err := repo.Reference(plumbing.ReferenceName("refs/heads/main"), true)
	if err != nil {
		return nil, fmt.Errorf("get local ref failed: %w", err)
	}

	// Get the latest commit from the local main branch
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("get commit object failed: %w", err)
	}

	return r.filterComposeFiles(*commit)
}

func (r DeploymentRepo) VerifyGitCli() error {
	cmd := exec.Command("git", "ls-remote")
	cmd.Dir = r.path

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git cli remote access failed: %w %s", err, out)
	}
	return nil
}

// TODO: Use go-git instead of exec when this issue is resolved (https://github.com/go-git/go-git/pull/1235)
func (r DeploymentRepo) Pull() error {
	cmd := exec.Command("git", "pull")
	cmd.Dir = r.path

	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(output))
		if outStr == "Already up to date." || outStr == "Already up-to-date." {
			return nil
		}
		return fmt.Errorf("pull failed: %w %s", err, output)
	}

	return nil
}
