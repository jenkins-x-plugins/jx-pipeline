package convert_test

import (
	"github.com/jenkins-x/go-scm/scm/driver/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner/fakerunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/convert"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = false

	lighthouseJenkinsXDir = filepath.Join(".lighthouse", "jenkins-x")
	packsDir              = filepath.Join("packs", "javascript", lighthouseJenkinsXDir)
	tasksDir              = filepath.Join("tasks", "javascript")
)

func TestConvertCatalog(t *testing.T) {
	srcDir := filepath.Join("test_data", "catalog", "pipeline-catalog")
	expectedDir := filepath.Join("test_data", "catalog", "expected")

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

	runner := &fakerunner.FakeRunner{}
	o.CommandRunner = runner.Run
	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = tmpDir
	o.Catalog = true

	err = o.Run()
	require.NoError(t, err, "Failed to run")

	if !generateTestOutput {
		testhelpers.AssertFilesEqualText(t, expectedDir, tmpDir,
			filepath.Join(packsDir, "pullrequest.yaml"),
			filepath.Join(packsDir, "release.yaml"),
			filepath.Join(tasksDir, "pullrequest.yaml"),
			filepath.Join(tasksDir, "release.yaml"),
		)
	}
}
func TestConvertRepository(t *testing.T) {
	srcDir := filepath.Join("test_data", "repo", "jx-cli")
	expectedDir := filepath.Join("test_data", "repo", "expected")

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

	scmClient, _ := fake.NewDefault()

	_, o := convert.NewCmdPipelineConvert()

	runner := &fakerunner.FakeRunner{}
	o.CommandRunner = runner.Run
	o.CatalogSHA = "myversionstreamref"
	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-cli"
	o.ScmOptions.ScmClient = scmClient
	o.ScmOptions.Dir = tmpDir

	err = o.Run()
	require.NoError(t, err, "Failed to run")

	if !generateTestOutput {
		testhelpers.AssertFilesEqualText(t, expectedDir, tmpDir,
			filepath.Join(lighthouseJenkinsXDir, "pullrequest.yaml"),
			filepath.Join(lighthouseJenkinsXDir, "release.yaml"),
		)
		assert.NoFileExists(t, filepath.Join(tmpDir, lighthouseJenkinsXDir, "Kptfile"))
	}
}
