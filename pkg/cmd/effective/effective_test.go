package effective_test

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/effective"
	"github.com/stretchr/testify/assert"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineEffective(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")
	expectedFile := filepath.Join(tmpDir, "pipeline.yaml")

	_, o := effective.NewCmdPipelineEffective()

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = "test_data"
	o.BatchMode = true
	o.OutFile = expectedFile
	err = o.Run()
	require.NoError(t, err, "Failed to run linter")

	assert.FileExists(t, expectedFile, "should have generated file")

	pr := &v1beta1.PipelineRun{}
	err = yamls.LoadFile(expectedFile, pr)
	require.NoError(t, err, "failed to parse PipelineRun from %s", expectedFile)

	t.Logf("generated valid YAML file %s\n", expectedFile)

}
