package lighthouses

import (
	"context"

	"github.com/jenkins-x/go-scm/scm"
)

type ScmProvider struct {
	ScmClient *scm.Client
	Ctx       context.Context
}

// NewScmProvider creates a new ScmProvider for working with lighthouse
func NewScmProvider(ctx context.Context, scmClient *scm.Client) *ScmProvider {
	return &ScmProvider{ScmClient: scmClient}
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
	answer, _, err := c.ScmClient.Contents.List(c.Ctx, fullName, filepath, commit)
	return answer, err
}

// ListFiles returns the files from git
func (c *ScmProvider) GetRepositoryByFullName(fullName string) (*scm.Repository, error) {
	answer, _, err := c.ScmClient.Repositories.Find(c.Ctx, fullName)
	return answer, err
}
