package pipelines

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/testpipelines"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/testhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/stretchr/testify/require"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	branchLabel   = "lighthouse.jenkins-x.io/branch"
	orgLabel      = "lighthouse.jenkins-x.io/refs.org"
	repoLabel     = "lighthouse.jenkins-x.io/refs.repo"
	buildNumLabel = "lighthouse.jenkins-x.io/buildNum"
	TestOrg       = "jenkins-x-plugins"
	PipelineRepo  = "jx-pipeline"
	SecretRepo    = "jx-secret"
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

func generatePipelineRunWithLabels(branch, org, repo, buildNum string) *v1beta1.PipelineRun {
	return &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-1",
			Labels: map[string]string{
				branchLabel:   branch,
				orgLabel:      org,
				repoLabel:     repo,
				buildNumLabel: buildNum,
			},
		},
	}
}

var paList = []v1.PipelineActivity{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "jenkins-x-plugins-jx-pipeline-master-2",
			Labels: map[string]string{
				branchLabel:   "master",
				orgLabel:      TestOrg,
				repoLabel:     PipelineRepo,
				buildNumLabel: "1601383238723",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "jenkins-x-plugins-jx-pipeline-master-5",
			Labels: map[string]string{
				branchLabel:   "master",
				orgLabel:      TestOrg,
				repoLabel:     PipelineRepo,
				buildNumLabel: "1601383238790",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "jenkins-x-plugins-jx-secret-master-4",
			Labels: map[string]string{
				branchLabel:   "master",
				orgLabel:      TestOrg,
				repoLabel:     SecretRepo,
				buildNumLabel: "1601383238723",
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "jenkins-x-plugins-jx-changelog-master-50",
			Labels: map[string]string{
				branchLabel:   "master",
				orgLabel:      TestOrg,
				repoLabel:     "jx-changelog",
				buildNumLabel: "1601383238724",
			},
		},
		Spec: v1.PipelineActivitySpec{
			Build: "50",
		},
	},
}

var BuildNumberTestCases = []struct {
	description          string
	pipelineRun          *v1beta1.PipelineRun
	paList               []v1.PipelineActivity
	expectedActivityName string
}{
	{
		"Create pa with build number one for first pipeline of a given prefix",
		generatePipelineRunWithLabels("master", TestOrg, "jx-promote", "16013832387908"),
		paList,
		"jenkins-x-plugins-jx-promote-master-1",
	},
	{
		"Do not Create pa if it already exists for a given prefix",
		generatePipelineRunWithLabels("master", TestOrg, "jx-changelog", "1601383238724"),
		paList,
		"jenkins-x-plugins-jx-changelog-master-50",
	},
	{
		"Create pa by incrementing higher build number for a given prefix - case 1",
		generatePipelineRunWithLabels("Master", TestOrg, PipelineRepo, "1601383238723"), // Check that the logic is case insensitive
		paList,
		"jenkins-x-plugins-jx-pipeline-master-6",
	},
	{
		"Create pa by incrementing higher build number for a given prefix - case 2",
		generatePipelineRunWithLabels("master", TestOrg, SecretRepo, "1601383238723"),
		paList,
		"jenkins-x-plugins-jx-secret-master-5",
	},
	{
		"Create pa for non master build",
		generatePipelineRunWithLabels("PR-120", TestOrg, SecretRepo, "1601383238723"),
		paList,
		"jenkins-x-plugins-jx-secret-pr-120-1",
	},
}

func TestPipelineBuildNumber(t *testing.T) {
	for _, tt := range BuildNumberTestCases {
		actualActivityName := ToPipelineActivityName(tt.pipelineRun, tt.paList)
		t.Log(tt.description)
		require.Equal(t, tt.expectedActivityName, actualActivityName)
	}
}

func BenchmarkPipelineBuildNumber(b *testing.B) {
	for n := 0; n < b.N; n++ {
		ToPipelineActivityName(generatePipelineRunWithLabels("master", TestOrg, "jx-test", "1601383238723"), paList)
	}
}
