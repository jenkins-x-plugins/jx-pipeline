package effective_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/effective"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/stretchr/testify/assert"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	"github.com/stretchr/testify/require"
)

var (
	gitConfigTemplate = `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
	ignorecase = true
	precomposeunicode = true
[remote "origin"]
	url = %s
	fetch = +refs/heads/*:refs/remotes/origin/*
`
)

func TestPipelineEffectiveJenkinsClientWithEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	actual := filepath.Join(tmpDir, "pipeline.yaml")
	expectedFile := filepath.Join("test_data", ".lighthouse", "jenkins-x", "expected-int.yaml")

	e := CreateTestEnvVars(t, tmpDir)
	e["GIT_URL"] = testGitURL
	backupVariables := currentEnv(e)
	setEnv(e)
	defer setEnv(backupVariables)

	_, o := effective.NewCmdPipelineEffective()

	o.Dir = "test_data"
	o.BatchMode = true
	o.AddDefaults = true
	o.OutFile = actual
	o.Resolver = CreateFakeResolver(t)
	err := o.Run()
	require.NoError(t, err, "Failed to run linter")

	assert.FileExists(t, actual, "should have generated file")

	pr := &v1beta1.PipelineRun{}
	err = yamls.LoadFile(actual, pr)
	require.NoError(t, err, "failed to parse PipelineRun from %s", actual)

	t.Logf("generated valid YAML file %s\n", actual)

	if generateTestOutput {
		err = files.CopyFile(actual, expectedFile)
		require.NoError(t, err, "failed to copy %s to %s", actual, expectedFile)
		return
	}
	testhelpers.AssertTextFileContentsEqual(t, actual, expectedFile)
}

func TestPipelineEffectiveJenkinsClientDiscoverGit(t *testing.T) {
	tmpDir := t.TempDir()
	actual := filepath.Join(tmpDir, "pipeline.yaml")
	expectedFile := filepath.Join("test_data", ".lighthouse", "jenkins-x", "expected-int.yaml")

	err := files.CopyDirOverwrite("test_data", tmpDir)
	require.NoError(t, err, "failed to copy test_data to %s", tmpDir)

	gitDir := filepath.Join(tmpDir, ".git")
	err = os.MkdirAll(gitDir, files.DefaultDirWritePermissions)
	require.NoError(t, err, "failed to make dirs %s", gitDir)

	gitConfig := filepath.Join(gitDir, "config")
	gitConfigText := fmt.Sprintf(gitConfigTemplate, testGitURL)
	err = ioutil.WriteFile(gitConfig, []byte(gitConfigText), files.DefaultFileWritePermissions)
	require.NoError(t, err, "failed to save file %s", gitConfig)

	e := CreateTestEnvVars(t, tmpDir)
	backupVariables := currentEnv(e)
	setEnv(e)
	defer setEnv(backupVariables)

	_, o := effective.NewCmdPipelineEffective()

	o.Dir = tmpDir
	o.BatchMode = true
	o.AddDefaults = true
	o.OutFile = actual
	o.Resolver = CreateFakeResolver(t)
	err = o.Run()
	require.NoError(t, err, "Failed to run linter")

	assert.FileExists(t, actual, "should have generated file")

	pr := &v1beta1.PipelineRun{}
	err = yamls.LoadFile(actual, pr)
	require.NoError(t, err, "failed to parse PipelineRun from %s", actual)

	t.Logf("generated valid YAML file %s\n", actual)

	if generateTestOutput {
		err = files.CopyFile(actual, expectedFile)
		require.NoError(t, err, "failed to copy %s to %s", actual, expectedFile)
		return
	}
	testhelpers.AssertTextFileContentsEqual(t, actual, expectedFile)
}

func CreateTestEnvVars(t *testing.T, tmpDir string) map[string]string {
	homeDir := filepath.Join(tmpDir, "home")
	xdgDir := filepath.Join(homeDir, "xdg")
	err := os.MkdirAll(xdgDir, files.DefaultDirWritePermissions)
	require.NoError(t, err, "failed to make dirs %s", xdgDir)

	e := map[string]string{
		"HOME":            homeDir,
		"XDG_CONFIG_HOME": xdgDir,

		// lets clear all the CI/CD env vars to avoid tests breaking inside CI
		"BUILD_ID":      "4",
		"BUILD_NUMBER":  "4",
		"BRANCH":        "main",
		"GIT_BRANCH":    "main",
		"GIT_COMMIT":    "abc1234",
		"JOB_NAME":      "myjob",
		"PULL_BASE_REF": "main",
		"PULL_PULL_SHA": "abc1234",
	}
	return e
}
func currentEnv(env map[string]string) map[string]string {
	e := map[string]string{}
	for k := range env {
		e[k] = os.Getenv(k)
	}
	return e
}

func setEnv(env map[string]string) {
	for k, v := range env {
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
	}
}
