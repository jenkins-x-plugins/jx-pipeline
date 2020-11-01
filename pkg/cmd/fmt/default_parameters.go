package fmt

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

var (
	// defaultParameterSpecs the default Lighthouse Pipeline Parameters which can be injected by the
	// lighthouse tekton engine
	defaultParameterSpecs = []v1beta1.ParamSpec{
		{
			Description: "the unique build number",
			Name:        "BUILD_ID",
			Type:        "string",
		},
		{
			Description: "the name of the job which is the trigger context name",
			Name:        "JOB_NAME",
			Type:        "string",
		},
		{
			Description: "the specification of the job",
			Name:        "JOB_SPEC",
			Type:        "string",
		},
		{
			Description: "'the kind of job: postsubmit or presubmit'",
			Name:        "JOB_TYPE",
			Type:        "string",
		},
		{
			Description: "the base git reference of the pull request",
			Name:        "PULL_BASE_REF",
			Type:        "string",
		},
		{
			Description: "the git sha of the base of the pull request",
			Name:        "PULL_BASE_SHA",
			Type:        "string",
		},
		{
			Description: "git pull request number",
			Name:        "PULL_NUMBER",
			Type:        "string",
		},
		{
			Description: "git pull request ref in the form 'refs/pull/$PULL_NUMBER/head'",
			Name:        "PULL_PULL_REF",
			Type:        "string",
		},
		{
			Description: "git revision to checkout (branch, tag, sha, ref…)",
			Name:        "PULL_PULL_SHA",
			Type:        "string",
		},
		{
			Description: "git pull reference strings of base and latest in the form 'master:$PULL_BASE_SHA,$PULL_NUMBER:$PULL_PULL_SHA:refs/pull/$PULL_NUMBER/head'",
			Name:        "PULL_REFS",
			Type:        "string",
		},
		{
			Description: "git repository name",
			Name:        "REPO_NAME",
			Type:        "string",
		},
		{
			Description: "git repository owner (user or organisation)",
			Name:        "REPO_OWNER",
			Type:        "string",
		},
		{
			Description: "git url to clone",
			Name:        "REPO_URL",
			Type:        "string",
		},
	}

	defaultParameterNames = createDefaultParameterNames()
)

func createDefaultParameterNames() map[string]bool {
	m := map[string]bool{}
	for _, dp := range defaultParameterSpecs {
		m[dp.Name] = true
	}
	m["subdirectory"] = true
	return m
}
