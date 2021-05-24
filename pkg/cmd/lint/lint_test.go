package lint_test

import (
	"path/filepath"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/lint"
	"github.com/stretchr/testify/require"
)

func TestLint(t *testing.T) {
	_, o := lint.NewCmdPipelineLint()

	o.Dir = filepath.Join("test_data", "valid")
	err := o.Run()
	require.NoError(t, err, "Failed to run linter")

	require.Len(t, o.Tests, 2, "resulting tests")
	for i := 0; i < 2; i++ {
		tr := o.Tests[i]
		require.NotNil(t, tr, "test result for %d", i)
		require.Nil(t, tr.Error, "error for test %d", i)
	}
}

func TestLintInvalid(t *testing.T) {
	_, o := lint.NewCmdPipelineLint()

	o.Dir = filepath.Join("test_data", "invalid")
	o.All = true
	err := o.Run()
	require.NoError(t, err, "Failed to run linter")

	require.Len(t, o.Tests, 1, "resulting tests")
	i := 0
	tr := o.Tests[i]
	require.NotNil(t, tr, "test result for %d", i)
	require.NotNil(t, tr.Error, "error for test %d", i)
	t.Logf("got expected error %v\n", tr.Error)
}
