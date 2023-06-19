package processor_test

import (
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines/processor"
	"github.com/stretchr/testify/assert"
)

func TestNewRefFromUsesImage(t *testing.T) {
	testCases := []struct {
		name              string
		image             string
		stepName          string
		reversionOverride string
		expected          *processor.GitRef
	}{
		{
			name:     "empty",
			image:    "",
			expected: nil,
		},
		{
			name:     "NonUsesImage",
			image:    "ghcr.io/jenkins-x/jx-boot:3.10.86",
			expected: nil,
		},
		{
			name:  "StandardJXPath",
			image: "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			expected: &processor.GitRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Revision:   "versionStream",
				Org:        "jenkins-x",
				Repository: "jx3-pipeline-catalog",
				PathInRepo: "tasks/go/pullrequest.yaml",
				IsPublic:   true,
			},
		},
		{
			name:  "GitlabPath",
			image: "uses:https://gitlab.com/open-source-archie/retry/tasks/go/pullrequest.yaml@versionStream",
			expected: &processor.GitRef{
				URL:        "https://gitlab.com/open-source-archie/retry.git",
				Org:        "open-source-archie",
				Repository: "retry",
				Revision:   "versionStream",
				PathInRepo: "tasks/go/pullrequest.yaml",
				IsPublic:   true,
			},
		},
		{
			name:              "OverrideRevision",
			image:             "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			reversionOverride: "master",
			expected: &processor.GitRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Org:        "jenkins-x",
				Repository: "jx3-pipeline-catalog",
				Revision:   "master",
				PathInRepo: "tasks/go/pullrequest.yaml",
				IsPublic:   true,
			},
		},
		{
			name:     "AddStepName",
			image:    "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			stepName: "build-make-build",
			expected: &processor.GitRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Org:        "jenkins-x",
				Repository: "jx3-pipeline-catalog",
				Revision:   "versionStream",
				PathInRepo: "tasks/go/pullrequest/build-make-build.yaml",
				IsPublic:   true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gitResolver := processor.NewGitRefResolver(tc.reversionOverride)
			actual, err := gitResolver.NewRefFromUsesImage(tc.image, tc.stepName)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
