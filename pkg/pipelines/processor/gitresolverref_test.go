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
		expected          processor.GitResolverRef
	}{
		{
			name:     "empty",
			image:    "",
			expected: processor.GitResolverRef{},
		},
		{
			name:     "NonUsesImage",
			image:    "ghcr.io/jenkins-x/jx-boot:3.10.86",
			expected: processor.GitResolverRef{},
		},
		{
			name:  "StandardJXPath",
			image: "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			expected: processor.GitResolverRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Revision:   "versionStream",
				PathInRepo: "tasks/go/pullrequest.yaml",
			},
		},
		{
			name:  "GitlabPath",
			image: "uses:https://gitlab.com/jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			expected: processor.GitResolverRef{
				URL:        "https://gitlab.com/jenkins-x/jx3-pipeline-catalog.git",
				Revision:   "versionStream",
				PathInRepo: "tasks/go/pullrequest.yaml",
			},
		},
		{
			name:              "OverrideRevision",
			image:             "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			reversionOverride: "master",
			expected: processor.GitResolverRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Revision:   "master",
				PathInRepo: "tasks/go/pullrequest.yaml",
			},
		},
		{
			name:     "AddStepName",
			image:    "uses:jenkins-x/jx3-pipeline-catalog/tasks/go/pullrequest.yaml@versionStream",
			stepName: "build-make-build",
			expected: processor.GitResolverRef{
				URL:        "https://github.com/jenkins-x/jx3-pipeline-catalog.git",
				Revision:   "versionStream",
				PathInRepo: "tasks/go/pullrequest/build-make-build.yaml",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := processor.NewRefFromUsesImage(tc.image, tc.stepName, tc.reversionOverride)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
