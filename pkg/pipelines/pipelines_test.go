package pipelines

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-pipeline/pkg/testpipelines"
	"github.com/stretchr/testify/require"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/yaml"
)

func TestInitialPipelineActivity(t *testing.T) {
	AssertPipelineActivityMapping(t, "initial")
}

func TestCreatePipelineActivity(t *testing.T) {
	AssertPipelineActivityMapping(t, "create")
}

func AssertPipelineActivityMapping(t *testing.T, folder string) {
	prFile := filepath.Join("test_data", folder, "pipelinerun.yaml")
	require.FileExists(t, prFile)

	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "failed to create temp dir")

	data, err := ioutil.ReadFile(prFile)
	require.NoError(t, err, "failed to load %s", prFile)

	pr := &v1beta1.PipelineRun{}
	err = yaml.Unmarshal(data, pr)
	require.NoError(t, err, "failed to unmarshal %s", prFile)

	pa := &v1.PipelineActivity{}
	ToPipelineActivity(pr, pa, false)

	testpipelines.ClearTimestamps(pa)

	paFile := filepath.Join(tmpDir, "pa.yaml")
	err = yamls.SaveFile(pa, paFile)
	require.NoError(t, err, "failed to save %s", paFile)

	t.Logf("created PipelineActivity %s\n", paFile)

	testhelpers.AssertTextFilesEqual(t, filepath.Join("test_data", folder, "expected.yaml"), paFile, "generated git credentials file")
}

func TestMergePipelineActivity(t *testing.T) {
	prFile := filepath.Join("test_data", "merge", "pipelinerun.yaml")
	require.FileExists(t, prFile)

	paFile := filepath.Join("test_data", "merge", "pa.yaml")
	require.FileExists(t, prFile)

	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "failed to create temp dir")

	pr := &v1beta1.PipelineRun{}
	err = yamls.LoadFile(prFile, pr)
	require.NoError(t, err, "failed to load %s", prFile)

	pa := &v1.PipelineActivity{}
	err = yamls.LoadFile(paFile, pa)
	require.NoError(t, err, "failed to load %s", paFile)

	ToPipelineActivity(pr, pa, false)

	testpipelines.ClearTimestamps(pa)

	paFile = filepath.Join(tmpDir, "pa.yaml")
	err = yamls.SaveFile(pa, paFile)
	require.NoError(t, err, "failed to save %s", paFile)

	t.Logf("created PipelineActivity %s\n", paFile)

	testhelpers.AssertTextFilesEqual(t, filepath.Join("test_data", "merge", "expected.yaml"), paFile, "generated git credentials file")
}
