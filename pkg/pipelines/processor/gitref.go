package processor

import (
	"path/filepath"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// GitRef is a representation of the Tekton git resolver params
type GitRef struct {
	URL        string
	Org        string
	Repository string
	Revision   string
	PathInRepo string
	IsPublic   bool
}

// GetParentFileName returns the filename of the parent pipeline run
func (g *GitRef) GetParentFileName() string {
	return strings.TrimSuffix(filepath.Base(g.PathInRepo), ".yaml")
}

// ToParams converts the GitRef to a slice of v1beta1.Param. If the GitRef is public, the URL param is used, otherwise
// the org and repo params are used
func (g *GitRef) ToParams() []v1beta1.Param {
	if g.IsPublic {
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
	return []v1beta1.Param{
		{
			Name:  "org",
			Value: v1beta1.ParamValue{StringVal: g.Org, Type: v1beta1.ParamTypeString},
		},
		{
			Name:  "repo",
			Value: v1beta1.ParamValue{StringVal: g.Repository, Type: v1beta1.ParamTypeString},
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

// ToResolverRef converts the GitRef to a v1beta1.ResolverRef
func (g *GitRef) ToResolverRef() v1beta1.ResolverRef {
	return v1beta1.ResolverRef{
		Resolver: "git",
		Params:   g.ToParams(),
	}
}
