package processor

import (
	"path/filepath"
	"strings"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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

// ToParams converts the GitRef to a slice of pipelinev1.Param. If the GitRef is public, the URL param is used, otherwise
// the org and repo params are used
func (g *GitRef) ToParams() []pipelinev1.Param {
	if g.IsPublic {
		return []pipelinev1.Param{
			{
				Name:  "url",
				Value: pipelinev1.ParamValue{StringVal: g.URL, Type: pipelinev1.ParamTypeString},
			},
			{
				Name:  "revision",
				Value: pipelinev1.ParamValue{StringVal: g.Revision, Type: pipelinev1.ParamTypeString},
			},
			{
				Name:  "pathInRepo",
				Value: pipelinev1.ParamValue{StringVal: g.PathInRepo, Type: pipelinev1.ParamTypeString},
			},
		}
	}
	return []pipelinev1.Param{
		{
			Name:  "org",
			Value: pipelinev1.ParamValue{StringVal: g.Org, Type: pipelinev1.ParamTypeString},
		},
		{
			Name:  "repo",
			Value: pipelinev1.ParamValue{StringVal: g.Repository, Type: pipelinev1.ParamTypeString},
		},
		{
			Name:  "revision",
			Value: pipelinev1.ParamValue{StringVal: g.Revision, Type: pipelinev1.ParamTypeString},
		},
		{
			Name:  "pathInRepo",
			Value: pipelinev1.ParamValue{StringVal: g.PathInRepo, Type: pipelinev1.ParamTypeString},
		},
	}
}

// ToResolverRef converts the GitRef to a pipelinev1.ResolverRef
func (g *GitRef) ToResolverRef() pipelinev1.ResolverRef {
	return pipelinev1.ResolverRef{
		Resolver: "git",
		Params:   g.ToParams(),
	}
}
