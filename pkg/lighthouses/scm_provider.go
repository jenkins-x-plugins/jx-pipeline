package lighthouses

import (
	"context"

	"github.com/jenkins-x/go-scm/scm"
)

type ScmProvider struct {
	ScmClient *scm.Client
}

// NewScmProvider creates a new ScmProvider for working with lighthouse
func NewScmProvider(scmClient *scm.Client) *ScmProvider {
	return &ScmProvider{ScmClient: scmClient}
}

// GetFile returns the file from git
func (c *ScmProvider) GetFile(owner, repo, filepath, commit string) ([]byte, error) {
	ctx := context.Background()
	fullName := scm.Join(owner, repo)
	answer, r, err := c.ScmClient.Contents.Find(ctx, fullName, filepath, commit)
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
	ctx := context.Background()
	fullName := scm.Join(owner, repo)
	answer, _, err := c.ScmClient.Contents.List(ctx, fullName, filepath, commit)
	return answer, err
}
