package lighthouses

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/lighthouse-client/pkg/filebrowser"
	"github.com/jenkins-x/lighthouse-client/pkg/scmprovider"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
)

// CreateResolver creates a new resolver
func CreateResolver(f *scmhelpers.Options) (*inrepo.UsesResolver, error) {
	err := f.Validate()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to discover scm client")
	}

	scmProvider := scmprovider.ToClient(f.ScmClient, "my-bot")
	fb := filebrowser.NewFileBrowserFromScmClient(scmProvider)

	fileBrowsers, err := filebrowser.NewFileBrowsers(f.GitServerURL, fb)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create file browsers")
	}

	return &inrepo.UsesResolver{
		FileBrowsers:     fileBrowsers,
		OwnerName:        f.Owner,
		RepoName:         f.Repository,
		Dir:              f.Dir,
		LocalFileResolve: true,
	}, nil
}
