package processor_test

import (
	"testing"

	"github.com/jenkins-x/jx-pipeline/pkg/pipelines/processor"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/stretchr/testify/assert"
)

func TestConvertLegacyStepAnnotationURLToUsesImage(t *testing.T) {
	key := inrepo.AppendStepURL
	testCases := []struct {
		text     string
		expected string
	}{
		{
			text:     "https://raw.githubusercontent.com/jenkins-x/jx3-pipeline-catalog/60bed6408732c1eda91a15713f51a9f97dcb1757/tasks/git-clone/git-clone-pr.yaml",
			expected: "uses:jenkins-x/jx3-pipeline-catalog/tasks/git-clone/git-clone-pr.yaml@versionStream",
		},
		{
			text:     "https://something/cheese.yaml",
			expected: "uses:https://something/cheese.yaml",
		},
		{
			text:     "",
			expected: "",
		},
	}
	for _, tc := range testCases {
		ann := map[string]string{
			key: tc.text,
		}
		actual := processor.ConvertLegacyStepAnnotationURLToUsesImage(ann, key)
		assert.Equal(t, tc.expected, actual, "for annotation value %s", tc.text)
		t.Logf("converted %s => %s\n", tc.text, actual)

		size := 0
		if actual == "" {
			size = 1
		}
		assert.Len(t, ann, size, "for annotation value %s", tc.text)
	}
}
