package pipelines

import (
	"context"
	"testing"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonfake "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// jx-build-controller crashed in production with a nil pointer deref because
// PipelineActivity.Spec.Steps had a Preview entry (written by jx-preview) at
// an index addTaskRunsMessage tried to write to. PipelineActivityStep.Stage
// is nil for non-Stage kinds, so positional indexing dereferenced nil.
func TestAddTaskRunsMessageSkipsNonStageSteps(t *testing.T) {
	pa := &v1.PipelineActivity{
		Spec: v1.PipelineActivitySpec{
			Steps: []v1.PipelineActivityStep{
				{
					Kind:    v1.ActivityStepKindTypePreview,
					Preview: &v1.PreviewActivityStep{ApplicationURL: "https://preview.example"},
				},
				{
					Kind: v1.ActivityStepKindTypeStage,
					Stage: &v1.StageActivityStep{
						CoreActivityStep: v1.CoreActivityStep{Name: "build"},
					},
				},
			},
		},
	}
	taskruns := []pipelinev1.TaskRun{
		{
			Status: pipelinev1.TaskRunStatus{
				Status: duckv1.Status{
					Conditions: []apis.Condition{
						{Message: "TaskRun build succeeded"},
					},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		addTaskRunsMessage(taskruns, pa)
	})

	assert.Equal(t, "Stage build succeeded", pa.Spec.Steps[1].Stage.Message.String())
}

func makeTerminatedTaskRun(name, ns, taskName string) *pipelinev1.TaskRun {
	finished := metav1.Time{Time: metav1.Now().Time}
	return &pipelinev1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"tekton.dev/pipelineTask": taskName},
		},
		Status: pipelinev1.TaskRunStatus{
			Status: duckv1.Status{
				Conditions: []apis.Condition{
					{Type: apis.ConditionSucceeded, Status: corev1.ConditionTrue},
				},
			},
			TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
				Steps: []pipelinev1.StepState{
					{
						Name: "step-main",
						ContainerState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode:   0,
								StartedAt:  finished,
								FinishedAt: finished,
							},
						},
					},
				},
			},
		},
	}
}

// Walk the real lifecycle that triggered the production bug:
//
//  1. The PipelineRun starts with the parallel tasks (frontend-tests,
//     backend-tests, build-and-deploy). jx-build-controller reconciles and
//     populates three Stage entries in the PipelineActivity.
//  2. Mid-build, build-and-deploy's `promote-jx-preview` step runs
//     `jx preview create`, which APPENDS a Preview step to pa.Spec.Steps.
//  3. frontend-tests + backend-tests complete; Tekton creates the sonar
//     TaskRun (it has runAfter on both). The PipelineRun's ChildReferences
//     now include sonar. jx-build-controller reconciles again.
//
// The bug was that step 3's reconcile inserted the new sonar Stage *before*
// the Preview using slice-aliasing-prone code, which overwrote the Preview
// slot and duplicated sonar. The dashboard then showed a phantom Running
// stage and lost the preview URL.
//
// This test reproduces the exact sequence and asserts that no Stage gets
// duplicated and the Preview (URL and all) survives.
func TestToPipelineActivityLifecycleDoesNotCorruptPreview(t *testing.T) {
	ns := "jx"
	pa := &v1.PipelineActivity{ObjectMeta: metav1.ObjectMeta{Namespace: ns}}
	client := tektonfake.NewSimpleClientset()

	// Step 1: parallel tasks running, sonar not yet created (Tekton hasn't
	// scheduled it because its runAfter prerequisites are not met yet).
	parallelRefs := []pipelinev1.ChildStatusReference{
		{Name: "tr-frontend-tests", PipelineTaskName: "frontend-tests"},
		{Name: "tr-backend-tests", PipelineTaskName: "backend-tests"},
		{Name: "tr-build-and-deploy", PipelineTaskName: "build-and-deploy"},
	}
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-test", Namespace: ns},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				ChildReferences: parallelRefs,
			},
		},
	}
	for _, ref := range parallelRefs {
		_, err := client.TektonV1().TaskRuns(ns).Create(
			context.Background(),
			makeTerminatedTaskRun(ref.Name, ns, ref.PipelineTaskName),
			metav1.CreateOptions{},
		)
		require.NoError(t, err)
	}

	// First reconcile: three Stage entries, no Preview yet.
	require.NoError(t, ToPipelineActivity(client, pr, pa, true))
	require.Len(t, pa.Spec.Steps, 3, "expected one Stage per parallel task")

	// Step 2: build-and-deploy's promote-jx-preview step runs `jx preview
	// create`, which APPENDS a Preview entry via GetOrCreatePreview. We
	// simulate that direct write here.
	pa.Spec.Steps = append(pa.Spec.Steps, v1.PipelineActivityStep{
		Kind: v1.ActivityStepKindTypePreview,
		Preview: &v1.PreviewActivityStep{
			Environment:    "preview",
			ApplicationURL: "https://alinapos-pr-404.preview.example",
			PullRequestURL: "https://gitlab.com/alinapos/alinapos/-/merge_requests/404",
		},
	})

	// Step 3: tests finish, Tekton schedules sonar, ChildReferences grows.
	pr.Status.ChildReferences = append(parallelRefs, pipelinev1.ChildStatusReference{
		Name: "tr-sonar", PipelineTaskName: "sonar",
	})
	_, err := client.TektonV1().TaskRuns(ns).Create(
		context.Background(),
		makeTerminatedTaskRun("tr-sonar", ns, "sonar"),
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	// Second reconcile: this is where the slice-aliasing bug fires today.
	require.NoError(t, ToPipelineActivity(client, pr, pa, true))

	var stageNames []string
	previewCount := 0
	var previewURL, previewPRURL string
	for _, s := range pa.Spec.Steps {
		switch s.Kind {
		case v1.ActivityStepKindTypeStage:
			require.NotNil(t, s.Stage)
			stageNames = append(stageNames, s.Stage.Name)
		case v1.ActivityStepKindTypePreview:
			previewCount++
			require.NotNil(t, s.Preview)
			previewURL = s.Preview.ApplicationURL
			previewPRURL = s.Preview.PullRequestURL
		}
	}
	assert.Equal(t,
		[]string{"frontend tests", "backend tests", "build and deploy", "sonar"},
		stageNames,
		"every PipelineTask must appear exactly once as a Stage")
	assert.Equal(t, 1, previewCount, "the Preview entry must not be lost or duplicated")
	assert.Equal(t,
		"https://alinapos-pr-404.preview.example",
		previewURL,
		"Preview ApplicationURL must survive the reconcile that adds sonar")
	assert.Equal(t,
		"https://gitlab.com/alinapos/alinapos/-/merge_requests/404",
		previewPRURL,
		"Preview PullRequestURL must survive the reconcile that adds sonar")
}

// Reconciliation must converge: once the build is fully done and the
// activity is up to date, running ToPipelineActivity again must produce a
// stable Spec.Steps. Otherwise repeated controller reconciles keep mutating
// the activity, accreting drift over time.
func TestToPipelineActivityIsIdempotent(t *testing.T) {
	ns := "jx"
	pa := &v1.PipelineActivity{ObjectMeta: metav1.ObjectMeta{Namespace: ns}}
	client := tektonfake.NewSimpleClientset()

	refs := []pipelinev1.ChildStatusReference{
		{Name: "tr-frontend-tests", PipelineTaskName: "frontend-tests"},
		{Name: "tr-backend-tests", PipelineTaskName: "backend-tests"},
		{Name: "tr-build-and-deploy", PipelineTaskName: "build-and-deploy"},
		{Name: "tr-sonar", PipelineTaskName: "sonar"},
	}
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-test", Namespace: ns},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{ChildReferences: refs},
		},
	}
	for _, ref := range refs {
		_, err := client.TektonV1().TaskRuns(ns).Create(
			context.Background(),
			makeTerminatedTaskRun(ref.Name, ns, ref.PipelineTaskName),
			metav1.CreateOptions{},
		)
		require.NoError(t, err)
	}
	pa.Spec.Steps = append(pa.Spec.Steps, v1.PipelineActivityStep{
		Kind: v1.ActivityStepKindTypePreview,
		Preview: &v1.PreviewActivityStep{
			ApplicationURL: "https://preview.example",
		},
	})

	require.NoError(t, ToPipelineActivity(client, pr, pa, true))
	first := pa.DeepCopy()

	require.NoError(t, ToPipelineActivity(client, pr, pa, true))

	require.Equal(t, len(first.Spec.Steps), len(pa.Spec.Steps),
		"Steps length must be stable across repeated reconciles")
	for i := range first.Spec.Steps {
		assert.Equal(t, first.Spec.Steps[i].Kind, pa.Spec.Steps[i].Kind,
			"Steps[%d].Kind diverged across reconciles", i)
	}
}
