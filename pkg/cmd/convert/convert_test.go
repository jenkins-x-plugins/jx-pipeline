package convert_test

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/convert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = false
)

func TestConvert(t *testing.T) {
	srcDir := filepath.Join("test_data", "pipeline-catalog")
	expectedDir := filepath.Join("test_data", "expected")

	var err error
	tmpDir := expectedDir
	if generateTestOutput {
		os.RemoveAll(expectedDir)
	} else {
		tmpDir, err = ioutil.TempDir("", "")
		require.NoError(t, err, "could not create temp dir")
	}

	err = files.CopyDir(srcDir, tmpDir, true)
	require.NoError(t, err, "failed to copy from %s to %s", srcDir, tmpDir)

	t.Logf("running tests in %s\n", tmpDir)

	_, o := convert.NewCmdPipelineConvert()

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = tmpDir
	o.Recursive = true

	err = o.Run()
	require.NoError(t, err, "Failed to run")

	if !generateTestOutput {
		files := []string{
			"packs/javascript/.lighthouse/jenkins-x/pullrequest.yaml",
			"packs/javascript/.lighthouse/jenkins-x/release.yaml",
			"tasks/javascript/pullrequest.yaml",
			"tasks/javascript/release.yaml",
		}
		for _, f := range files {
			generated := filepath.Join(tmpDir, f)
			expected := filepath.Join(expectedDir, f)
			testhelpers.AssertEqualFileText(t, expected, generated)
			t.Logf("generated file %s matches expected\n", f)
		}
	}
}
