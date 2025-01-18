package lighthouses

import (
	"context"
	"fmt"
	"strings"

	"github.com/jenkins-x/go-scm/scm"
)

type ScmProvider struct {
	ScmClient *scm.Client
	Ctx       context.Context
}

// NewScmProvider creates a new ScmProvider for working with lighthouse
func NewScmProvider(ctx context.Context, scmClient *scm.Client) *ScmProvider {
	return &ScmProvider{ScmClient: scmClient, Ctx: ctx}
}

// GetFile returns the file from git
func (c *ScmProvider) GetFile(owner, repo, filepath, commit string) ([]byte, error) {
	fullName := scm.Join(owner, repo)
	answer, r, err := c.ScmClient.Contents.Find(c.Ctx, fullName, filepath, commit)
	// handle files not existing nicely
	if r != nil && r.Status == 404 {
		return nil, nil
	}
	var data []byte
	if answer != nil {
		data = answer.Data
	}
	return data, err
}

// ListFiles returns the files from git
func (c *ScmProvider) ListFiles(owner, repo, filepath, commit string) ([]*scm.FileEntry, error) {
	fullName := scm.Join(owner, repo)
	answer, _, err := c.ScmClient.Contents.List(c.Ctx, fullName, filepath, commit, &scm.ListOptions{})
	return answer, err
}

// ListFiles returns the files from git
func (c *ScmProvider) GetRepositoryByFullName(fullName string) (*scm.Repository, error) {
	answer, _, err := c.ScmClient.Repositories.Find(c.Ctx, fullName)
	return answer, err
}

// GetMainAndCurrentBranchRefs find the main branch
func (c *ScmProvider) GetMainAndCurrentBranchRefs(owner, repo, eventRef string) ([]string, error) {
	fullName := scm.Join(owner, repo)
	repository, _, err := c.ScmClient.Repositories.Find(c.Ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("failed to find repository %s: %w", fullName, err)
	}
	mainBranch := repository.Branch
	if mainBranch == "" {
		mainBranch = "master"
	}

	refs := []string{mainBranch}

	eventRef = strings.TrimPrefix(eventRef, "refs/heads/")
	eventRef = strings.TrimPrefix(eventRef, "refs/tags/")
	if eventRef != mainBranch && eventRef != "" {
		refs = append(refs, eventRef)
	}
	return refs, nil
}
