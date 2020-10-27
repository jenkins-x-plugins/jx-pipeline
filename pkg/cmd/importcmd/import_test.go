package importcmd_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/importcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineImport(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")

	srcDir := "test_data"
	err = files.CopyDir(srcDir, dir, true)
	require.NoError(t, err, "failed to copy from %s to %s", srcDir, dir)

	t.Logf("running tests in dir %s\n", dir)

	_, o := importcmd.NewCmdPipelineImport()
	o.Dir = dir
	o.BatchMode = true
	o.TaskFolder = "buildpacks"
	o.TaskVersion = "0.1"

	g := o.Git()
	err = gitclient.Init(g, dir)
	require.NoError(t, err, "failed git init in %s", dir)

	err = gitclient.Add(g, dir, "*")
	require.NoError(t, err, "failed to add files to git in %s", srcDir, dir)

	err = o.Run()
	require.NoError(t, err, "failed to import pipeline in %s", dir)

	outDir := filepath.Join(dir, ".lighthouse", o.TaskFolder)

	assert.FileExists(t, filepath.Join(outDir, "triggers.yaml"), "should have lighthouse triggers file")
	assert.FileExists(t, filepath.Join(outDir, o.TaskFolder+".yaml"), "should have tekton Task file")
}
