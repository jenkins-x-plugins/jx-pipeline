package processor

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
)

type GitRefResolver struct {
	isPublicRepositories map[string]bool
	reversionOverride    string
}

func NewGitRefResolver(reversionOverride string) *GitRefResolver {
	return &GitRefResolver{reversionOverride: reversionOverride, isPublicRepositories: map[string]bool{}}
}

// NewRefFromUsesImage parses an image name of "uses:image" syntax as a GitRef
func (g *GitRefResolver) NewRefFromUsesImage(image, stepName string) (*GitRef, error) {
	if !strings.HasPrefix(image, "uses:") {
		return nil, nil
	}
	image = strings.TrimPrefix(image, "uses:")
	split := strings.Split(image, "@")

	fullURL, revision := split[0], split[1]
	if g.reversionOverride != "" {
		revision = g.reversionOverride
	}

	if !strings.HasPrefix(fullURL, "https://") {
		// If the URL doesn't start with https:// then we can assume that it is a GitHub URL and so prepend that domain
		fullURL = fmt.Sprintf("%s/%s", giturl.GitHubURL, fullURL)
	}

	split = strings.SplitAfter(fullURL, "/")
	repoURL := strings.Join(split[:5], "")
	org, repo := strings.TrimSuffix(split[3], "/"), strings.TrimSuffix(split[4], "/")
	repoURL = strings.TrimSuffix(repoURL, "/") + ".git"

	isPublic, ok := g.isPublicRepositories[repoURL]
	if !ok {
		var err error
		isPublic, err = g.isRepositoryPublic(repoURL)
		if err != nil {
			return nil, err
		}
		g.isPublicRepositories[repoURL] = isPublic
	}

	pathInRepo := strings.Join(split[5:], "")
	if stepName != "" {
		pathInRepo = filepath.Join(strings.TrimSuffix(pathInRepo, ".yaml"), stepName) + ".yaml"
	}

	return &GitRef{
		URL:        repoURL,
		Org:        org,
		Repository: repo,
		Revision:   revision,
		PathInRepo: pathInRepo,
		IsPublic:   isPublic,
	}, nil
}

func (g *GitRefResolver) isRepositoryPublic(cloneURL string) (bool, error) {
	client := &http.Client{}
	req, err := http.NewRequest("HEAD", cloneURL, &bytes.Buffer{})
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}
