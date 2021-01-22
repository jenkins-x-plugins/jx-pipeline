package convert_test

import (
	"github.com/jenkins-x/go-scm/scm/driver/fake"
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

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-pipeline"
	o.ScmOptions.Dir = tmpDir
	o.Catalog = true

	err = o.Run()
	require.NoError(t, err, "Failed to run")

	if !generateTestOutput {
		AssertFilesEqualText(t, expectedDir, tmpDir,
			filepath.Join(packsDir, "pullrequest.yaml"),
			filepath.Join(packsDir, "release.yaml"),
			filepath.Join(tasksDir, "pullrequest.yaml"),
			filepath.Join(tasksDir, "release.yaml"),
		)
	}
}

// AssertFilesEqualText asserts that all the given paths in the expected dir are equal to the files in the actual dir
func AssertFilesEqualText(t *testing.T, expectedDir string, actualDir string, paths ...string) {
	for _, f := range paths {
		generated := filepath.Join(actualDir, f)
		expected := filepath.Join(expectedDir, f)
		testhelpers.AssertEqualFileText(t, expected, generated)
		t.Logf("generated file %s matches expected\n", f)
	}
}

func TestConvertRepository(t *testing.T) {
	os.Setenv("LIGHTHOUSE_VERSIONSTREAM_JENKINS_X_JX3_PIPELINE_CATALOG", "myversionstreamref")

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

	o.ScmOptions.SourceURL = "https://github.com/jenkins-x/jx-cli"
	o.ScmOptions.ScmClient = scmClient
	o.ScmOptions.Dir = tmpDir

	err = o.Run()
	require.NoError(t, err, "Failed to run")

	if !generateTestOutput {
		AssertFilesEqualText(t, expectedDir, tmpDir,
			filepath.Join(lighthouseJenkinsXDir, "pullrequest.yaml"),
			filepath.Join(lighthouseJenkinsXDir, "release.yaml"),
		)
		assert.NoFileExists(t, filepath.Join(tmpDir, lighthouseJenkinsXDir, "Kptfile"))
	}
}
