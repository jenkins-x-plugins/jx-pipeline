package convert_test

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/convert"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvert(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")

	err = files.CopyDir("test_data", tmpDir, true)
	require.NoError(t, err, "failed to copy from test_data to %s", tmpDir)

	t.Logf("running tests in %s\n", tmpDir)

	_, o := convert.NewCmdPipelineConvert()

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = tmpDir
	o.Recursive = true

	err = o.Run()
	require.NoError(t, err, "Failed to run")
}
