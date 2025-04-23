package git

import (
	"fmt"
	"os"
	"path"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

var (
	ErrPathDoesNotExist = fmt.Errorf("path does not exist")
	ErrNotAGitRepo = fmt.Errorf("not a git repository")
	ErrOpenRepoFailed = fmt.Errorf("open repo failed")
	ErrGetWorktreeFailed = fmt.Errorf("get worktree failed")
	ErrGetStatusFailed = fmt.Errorf("get status failed")
	ErrHasLocalChanges = fmt.Errorf("local changes detected")
	ErrFetchFailed = fmt.Errorf("fetch failed")
	ErrGetLocalRefFailed = fmt.Errorf("get local ref failed")
	ErrGetRemoteRefFailed = fmt.Errorf("get remote ref failed")
	ErrGetCommitObjectFailed = fmt.Errorf("get commit object failed")
	ErrGetTreeFailed = fmt.Errorf("get tree failed")
	ErrWalkTreeFailed = fmt.Errorf("walk tree failed")
	ErrPullFailed = fmt.Errorf("pull failed")
)

type DeploymentRepo struct {
	auth *gitHttp.BasicAuth
	path string
}

func NewDeploymentRepo(username, password, path string) (*DeploymentRepo, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ErrPathDoesNotExist
	}

	_, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, ErrNotAGitRepo
	}

	return &DeploymentRepo{
		auth: &gitHttp.BasicAuth{
			Username: username,
			Password: password,
		},
		path: path,
	}, nil
}

func (r DeploymentRepo) HasChanges() (bool, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return false, ErrOpenRepoFailed
	}

	// Get the working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return false, ErrGetWorktreeFailed
	}

	// Check if the working tree is clean
	status, err := worktree.Status()
	if err != nil {
		return false, ErrGetStatusFailed
	}

	// If there are changes, we cannot savely proceed
	if !status.IsClean() {
		return false, ErrHasLocalChanges
	}

	// Fetch the latest changes from the remote repository
    err = repo.Fetch(&gogit.FetchOptions{
		RemoteName: "origin",
        Auth: r.auth,
		Tags: gogit.NoTags,
		Force: true,
	})
	if err != nil {
		if (err == gogit.NoErrAlreadyUpToDate) {
			return false, nil
		}
		return false, ErrFetchFailed
	}

	// Get the local references for the main branch
    localRef, err := repo.Reference(plumbing.ReferenceName("refs/heads/main"), true)
	if err != nil {
		return false, ErrGetLocalRefFailed
	}

	// Get the remote references for the main branch
    remoteRef, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return false, ErrGetRemoteRefFailed
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
		return nil, ErrGetTreeFailed
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
        return nil, ErrWalkTreeFailed
    }
	
	return composeFiles, nil
}

func (r DeploymentRepo) GetRemoteComposeFiles() ([]string, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return nil, ErrOpenRepoFailed
	}

	// Get the remote references for the main branch
	ref, err := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/main"), true)
	if err != nil {
		return nil, ErrGetRemoteRefFailed
	}

	// Get the latest commit from the remote main branch
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, ErrGetCommitObjectFailed
	}

	return r.filterComposeFiles(*commit)
}

func (r DeploymentRepo) GetLocalComposeFiles() ([]string, error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return nil, ErrOpenRepoFailed
	}

	// Get the local references for the main branch
	ref, err := repo.Reference(plumbing.ReferenceName("refs/heads/main"), true)
	if err != nil {
		return nil, ErrGetLocalRefFailed
	}

	// Get the latest commit from the local main branch
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, ErrGetCommitObjectFailed
	}

	return r.filterComposeFiles(*commit)
}

func (r DeploymentRepo) Pull() (error) {
	// Open the repository
	repo, err := gogit.PlainOpen(r.path)
	if err != nil {
		return ErrOpenRepoFailed
	}

	// Get the working tree
	w, err := repo.Worktree()
	if err != nil {
		return ErrGetWorktreeFailed
	}

	// Pull the latest changes from the remote repository
	err = w.Pull(&gogit.PullOptions{
		RemoteName: "origin",
		Auth: r.auth,
		SingleBranch: true,
	})
	if err != nil {
		if err == gogit.NoErrAlreadyUpToDate {
			return nil
		}
		return ErrPullFailed
	}

	return nil
}