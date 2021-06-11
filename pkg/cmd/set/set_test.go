package set_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/set"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = false
)

func TestPipelineSet(t *testing.T) {
	_, o := set.NewCmdPipelineSet()

	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "failed to create tmp dir")

	srcDir := "test_data"

	err = files.CopyDirOverwrite(srcDir, tmpDir)
	require.NoError(t, err, "failed to copy test files at %s to %s", srcDir, tmpDir)

	o.Dir = tmpDir
	o.TemplateEnvs = []string{"HOME=/tekton/home"}

	err = o.Run()
	require.NoError(t, err, "failed to run in dir %s", tmpDir)

	expectedPath := filepath.Join(srcDir, "cheese", "expected.yaml")
	generatedFile := filepath.Join(tmpDir, "cheese", "release.yaml")

	if generateTestOutput {
		data, err := ioutil.ReadFile(generatedFile)
		require.NoError(t, err, "failed to load %s", generatedFile)

		err = ioutil.WriteFile(expectedPath, data, 0666)
		require.NoError(t, err, "failed to save file %s", expectedPath)

		t.Logf("saved file %s\n", expectedPath)
		return
	}

	testhelpers.AssertTextFilesEqual(t, expectedPath, generatedFile, "generated file")
}
