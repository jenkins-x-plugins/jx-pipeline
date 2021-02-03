package override_test

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/jenkins-x/jx-pipeline/pkg/cmd/override"
	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = false
)

func TestPipelineOverride(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")

	expectedFile := filepath.Join("test_data", "expected.yaml")

	err = files.CopyDirOverwrite("test_data", tmpDir)
	require.NoError(t, err, "failed to copy test_data to %s", tmpDir)

	_, o := override.NewCmdPipelineOverride()
	o.Dir = tmpDir
	o.BatchMode = true
	o.CatalogSHA = "7a05c45bafc60e0571509526d91ed5963e4c2d54"
	o.PipelineName = "postsubmit/release"
	o.Step = "build-container-build"
	err = o.Run()
	require.NoError(t, err, "Failed to run linter")

	actual := filepath.Join(tmpDir, ".lighthouse", "jenkins-x", "release.yaml")
	if generateTestOutput {
		err = files.CopyFile(actual, expectedFile)
		require.NoError(t, err, "failed to copy %s to %s", actual, expectedFile)
		return
	}
	testhelpers.AssertTextFileContentsEqual(t, actual, expectedFile)

}
