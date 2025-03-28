package pipelines_test

import (
	"path/filepath"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/testpipelines"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/stretchr/testify/require"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	branchLabel   = "lighthouse.jenkins-x.io/branch"
	orgLabel      = "lighthouse.jenkins-x.io/refs.org"
	repoLabel     = "lighthouse.jenkins-x.io/refs.repo"
	buildNumLabel = "lighthouse.jenkins-x.io/buildNum"
	TestOrg       = "jenkins-x-plugins"
	TestOrgSecond = "jenkins-x"
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
	// getting path of prfile
	prFile := filepath.Join("testdata", folder, "pipelinerun.yaml")
	require.FileExists(t, prFile)

	// loading a prfile
	pr := &pipelinev1.PipelineRun{}
	err := yamls.LoadFile(prFile, pr)
	require.NoError(t, err, "failed to unmarshal %s", prFile)

	// generating pa from pr
	pa := &v1.PipelineActivity{}
	pipelines.ToPipelineActivity(pr, pa, false)

	// removing timestamps
	testpipelines.ClearTimestamps(pa)

	// expected pa
	expectedFile := filepath.Join("testdata", folder, "expected.yaml")
	require.FileExists(t, expectedFile)

	expectedPa := &v1.PipelineActivity{}
	err = yamls.LoadFile(expectedFile, expectedPa)
	require.NoError(t, err, "failed to load %s", expectedPa)

	require.Equal(t, expectedPa, pa)
}

func TestMergePipelineActivity(t *testing.T) {
	// copying pr path
	prFile := filepath.Join("testdata", "merge", "pipelinerun.yaml")
	require.FileExists(t, prFile)

	// copying expected pa path
	expectedFile := filepath.Join("testdata", "merge", "expected.yaml")
	require.FileExists(t, expectedFile)

	// loading the pr
	pr := &pipelinev1.PipelineRun{}
	err := yamls.LoadFile(prFile, pr)
	require.NoError(t, err, "failed to load %s", prFile)

	// loading expectedPa
	expectedPa := &v1.PipelineActivity{}
	err = yamls.LoadFile(expectedFile, expectedPa)
	require.NoError(t, err, "failed to load %s", expectedPa)

	// creating pa from pr
	pa := &v1.PipelineActivity{}
	pipelines.ToPipelineActivity(pr, pa, false)

	// removing the timestamp from pa
	testpipelines.ClearTimestamps(pa)

	// compare the pa with expected.yaml (expectedPa)
	require.Equal(t, expectedPa, pa)
}

func generatePipelineRunWithLabels(branch, org, repo, buildNum string) *pipelinev1.PipelineRun {
	return &pipelinev1.PipelineRun{
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
			Name: "jenkins-x-plugins-jx-pipeline-pr-404-1",
			Labels: map[string]string{
				branchLabel:   "PR-404",
				orgLabel:      TestOrg,
				repoLabel:     PipelineRepo,
				buildNumLabel: "1601383238723",
			},
		},
	},
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
	pipelineRun          *pipelinev1.PipelineRun
	paList               []v1.PipelineActivity
	expectedActivityName string
}{
	{
		"Create pa with build number 2 for second pipeline of a given prefix",
		generatePipelineRunWithLabels("pr-404", TestOrg, "jx-pipeline", "16013832387908"),
		paList,
		"jenkins-x-plugins-jx-pipeline-pr-404-2",
	},
	{
		"Create pa with build number one for first pipeline of a given prefix",
		generatePipelineRunWithLabels("master", TestOrgSecond, "jx-promote", "16013832387908"),
		paList,
		"jenkins-x-jx-promote-master-1",
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
		actualActivityName := pipelines.ToPipelineActivityName(tt.pipelineRun, tt.paList)
		t.Log(tt.description)
		require.Equal(t, tt.expectedActivityName, actualActivityName)
	}
}

func BenchmarkPipelineBuildNumber(b *testing.B) {
	for n := 0; n < b.N; n++ {
		pipelines.ToPipelineActivityName(generatePipelineRunWithLabels("master", TestOrg, "jx-test", "1601383238723"), paList)
	}
}

var activityStatusTestCases = []struct {
	description    string
	folder         string
	expectedStatus string
}{
	{
		description:    "Tekton pipeline run has timed out and has no steps",
		folder:         "timeout-with-no-steps",
		expectedStatus: v1.ActivityStatusTypeTimedOut.String(),
	},
	{
		description:    "Tekton pipeline run has timed out and has steps",
		folder:         "timeout-with-steps",
		expectedStatus: v1.ActivityStatusTypeTimedOut.String(),
	},
	{
		description:    "Tekton pipeline run has failed and has no steps",
		folder:         "failed",
		expectedStatus: v1.ActivityStatusTypeFailed.String(),
	},
	{
		description:    "Tekton pipeline run has been cancelled and has no steps",
		folder:         "cancel-with-no-steps",
		expectedStatus: v1.ActivityStatusTypeCancelled.String(),
	},
	{
		description:    "Tekton pipeline run has been cancelled and has steps",
		folder:         "cancel-with-steps",
		expectedStatus: v1.ActivityStatusTypeCancelled.String(),
	},
}

func TestPipelineActivityStatus(t *testing.T) {
	for k, v := range activityStatusTestCases {
		t.Logf("Running test case %d: %s", k, v.description)
		prFile := filepath.Join("testdata", v.folder, "pr.yaml")
		require.FileExists(t, prFile)

		pr := &pipelinev1.PipelineRun{}
		err := yamls.LoadFile(prFile, pr)
		require.NoError(t, err, "failed to load %s", prFile)

		pa := &v1.PipelineActivity{}

		pipelines.ToPipelineActivity(pr, pa, false)
		require.Equal(t, v.expectedStatus, pa.Spec.Status.String())
	}
}

var activityMessageTestCases = []struct {
	description     string
	folder          string
	name            string
	expectedMessage string
}{
	{
		description:     "Jenkins X PipelineActivity has been successful run and message exists",
		folder:          "message/success",
		name:            "jx-test-project-repo-pr-1-7",
		expectedMessage: `Tasks Completed: 1 (Failed: 0, Cancelled 0), Skipped: 0`,
	},
	{
		description:     "Jenkins X PipelineActivity has been timedout and message exists",
		folder:          "message/timeout",
		name:            "jx-test-project-repo-pr-1-7",
		expectedMessage: `PipelineActivity "jx-test-project-repo-pr-1-7" failed to finish within "1h0m0s"`,
	},
	{
		description:     "Jenkins X PipelineActivity has been cancelled and message exists",
		folder:          "message/cancelled",
		name:            "jx-test-project-repo-pr-2-7",
		expectedMessage: `PipelineActivity "jx-test-project-repo-pr-2-7" was cancelled`,
	},
	{
		description:     "Jenkins X PipelineActivity has two steps, one step that been timedout and other Succeeded",
		folder:          "message/timedout-succeeded",
		name:            "jx-test-project-repo-pr-2-7",
		expectedMessage: `PipelineActivity "jx-test-project-repo-pr-2-7" failed to finish within "1m0s"`,
	},
}

func TestPipelineActivityMessage(t *testing.T) {
	for k, v := range activityMessageTestCases {
		t.Logf("Running test case %d: %s", k+1, v.description)
		prFile := filepath.Join("testdata", v.folder, "pr.yaml")
		require.FileExists(t, prFile)

		pr := &pipelinev1.PipelineRun{}
		err := yamls.LoadFile(prFile, pr)
		require.NoError(t, err, "failed to load %s", prFile)

		pa := &v1.PipelineActivity{}

		pa.Name = v.name
		pipelines.ToPipelineActivity(pr, pa, false)

		for k2 := range pa.Spec.Steps {
			require.NotEqual(t, "", pa.Spec.Steps[k2].Stage.Message.String())
		}

		require.NotEqual(t, "", pa.Spec.Message.String())
		require.Equal(t, v.expectedMessage, pa.Spec.Message.String())
	}
}
