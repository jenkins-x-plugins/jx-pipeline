package lint_test

import (
	"testing"

	"github.com/jenkins-x/jx-pipeline/pkg/cmd/lint"
	"github.com/stretchr/testify/require"
)

func TestLint(t *testing.T) {
	_, o := lint.NewCmdPipelineLint()

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = "test_data"
	err := o.Run()
	require.NoError(t, err, "Failed to run linter")

	require.Len(t, o.Tests, 2, "resulting tests")
	for i := 0; i < 2; i++ {
		tr := o.Tests[i]
		require.NotNil(t, tr, "test result for %d", i)
		require.Nil(t, tr.Error, "error for test %d", i)
	}
}
