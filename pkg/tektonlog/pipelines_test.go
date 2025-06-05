package tektonlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apis "knative.dev/pkg/apis"
)

const (
	ns = "jx"
)

func TestPipelineRunIsNotPendingCompletedRun(t *testing.T) {
	now := metav1.Now()
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "CompletedPR",
			Namespace: ns,
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				CompletionTime: &now,
			},
		},
	}

	assert.True(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsPendingCondition(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PendingPR",
			Namespace: ns,
		},
	}

	pr.Status.SetCondition(&apis.Condition{
		Status: "Unknown",
		Reason: "Pending",
	})

	assert.False(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingRunningCondition(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "RunningPR",
			Namespace: ns,
		},
	}

	pr.Status.SetCondition(&apis.Condition{
		Status: "Unknown",
		Reason: "Running",
	})

	assert.True(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingSucceededCondition(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "SucceededPR",
			Namespace: "jx",
		},
	}

	pr.Status.SetCondition(&apis.Condition{
		Status: "True",
		Reason: "Succeeded",
	})

	assert.True(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingNoConditions(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoConditionPR",
			Namespace: ns,
		},
		Status: pipelinev1.PipelineRunStatus{},
	}

	assert.True(t, PipelineRunIsNotPending(pr))
}
