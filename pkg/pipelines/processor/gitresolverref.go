package processor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// GitResolverRef is a representation of the Tekton git resolver params
type GitResolverRef struct {
	URL        string
	Revision   string
	PathInRepo string
}

// NewRefFromUsesImage parses an image name of "uses:image" syntax as a GitResolverRef
func NewRefFromUsesImage(image, stepName, reversionOverride string) GitResolverRef {
	if !strings.HasPrefix(image, "uses:") {
		return GitResolverRef{}
	}
	image = strings.TrimPrefix(image, "uses:")
	split := strings.Split(image, "@")

	fullURL, revision := split[0], split[1]
	if reversionOverride != "" {
		revision = reversionOverride
	}

	if !strings.HasPrefix(fullURL, "https://") {
		// If the URL doesn't start with https:// then we can assume that it is a GitHub URL and so prepend that domain
		fullURL = fmt.Sprintf("https://github.com/%s", fullURL)
	}

	split = strings.SplitAfter(fullURL, "/")
	repoURL := strings.Join(split[:5], "")
	repoURL = strings.TrimSuffix(repoURL, "/") + ".git"

	pathInRepo := strings.Join(split[5:], "")
	if stepName != "" {
		pathInRepo = filepath.Join(strings.TrimSuffix(pathInRepo, ".yaml"), stepName) + ".yaml"
	}

	return GitResolverRef{
		URL:        repoURL,
		Revision:   revision,
		PathInRepo: pathInRepo,
	}
}

// IsEmpty returns true if all the fields are empty
func (g GitResolverRef) IsEmpty() bool {
	return g.URL == "" && g.Revision == "" && g.PathInRepo == ""
}

// GetParentFileName returns the filename of the parent pipeline run
func (g GitResolverRef) GetParentFileName() string {
	return strings.TrimSuffix(filepath.Base(g.PathInRepo), ".yaml")
}

// ToParams converts the GitResolverRef to a slice of v1beta1.Param
func (g GitResolverRef) ToParams() []v1beta1.Param {
	return []v1beta1.Param{
		{
			Name:  "url",
			Value: v1beta1.ParamValue{StringVal: g.URL, Type: v1beta1.ParamTypeString},
		},
		{
			Name:  "revision",
			Value: v1beta1.ParamValue{StringVal: g.Revision, Type: v1beta1.ParamTypeString},
		},
		{
			Name:  "pathInRepo",
			Value: v1beta1.ParamValue{StringVal: g.PathInRepo, Type: v1beta1.ParamTypeString},
		},
	}
}

// ToResolverRef converts the GitResolverRef to a v1beta1.ResolverRef
func (g GitResolverRef) ToResolverRef() v1beta1.ResolverRef {
	return v1beta1.ResolverRef{
		Resolver: "git",
		Params:   g.ToParams(),
	}
}
