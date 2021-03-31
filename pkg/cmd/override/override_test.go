package override_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/jenkins-x/lighthouse-client/pkg/filebrowser"
	"github.com/jenkins-x/lighthouse-client/pkg/filebrowser/fake"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"

	"github.com/jenkins-x/jx-pipeline/pkg/cmd/override"
	"github.com/stretchr/testify/require"
)

var (
	// generateTestOutput enable to regenerate the expected output
	generateTestOutput = false
)

func TestPipelineOverrideStep(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")

	srcDir := filepath.Join("test_data", "step")
	expectedFile := filepath.Join(srcDir, "expected.yaml")

	err = files.CopyDirOverwrite(srcDir, tmpDir)
	require.NoError(t, err, "failed to copy %s to %s", srcDir, tmpDir)

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

func TestPipelineOverrideTask(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err, "could not create temp dir")

	srcDir := filepath.Join("test_data", "task")
	expectedFile := filepath.Join(srcDir, "expected.yaml")

	filebrowsers, err := filebrowser.NewFileBrowsers(giturl.GitHubURL, fake.NewFakeFileBrowser(filepath.Join("test_data", "fake_file_browser")))
	require.NoError(t, err, "failed to create file browsers")

	actual := filepath.Join(tmpDir, "pipeline.yaml")

	err = files.CopyDirOverwrite(srcDir, tmpDir)
	require.NoError(t, err, "failed to copy %s to %s", srcDir, tmpDir)

	_, o := override.NewCmdPipelineOverride()
	o.Dir = tmpDir
	o.BatchMode = true
	o.CatalogSHA = "7a05c45bafc60e0571509526d91ed5963e4c2d54"
	o.File = actual
	o.Resolver = &inrepo.UsesResolver{
		FileBrowsers:     filebrowsers,
		OwnerName:        "myorg",
		LocalFileResolve: true,
	}
	err = o.Run()
	require.NoError(t, err, "Failed to run linter")

	if generateTestOutput {
		err = files.CopyFile(actual, expectedFile)
		require.NoError(t, err, "failed to copy %s to %s", actual, expectedFile)
		return
	}
	testhelpers.AssertTextFileContentsEqual(t, actual, expectedFile)
}
