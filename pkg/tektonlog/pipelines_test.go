//go:build unit
// +build unit

package tektonlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				Conditions: corev1.Conditions{
					{
						Type:   corev1.ConditionSucceeded,
						Status: corev1.ConditionUnknown,
						Reason: "Pending",
					},
				},
			},
		},
	}

	assert.False(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingRunningCondition(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "RunningPR",
			Namespace: ns,
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				Conditions: corev1.Conditions{
					{
						Type:   corev1.ConditionSucceeded,
						Status: corev1.ConditionUnknown,
						Reason: "Running",
					},
				},
			},
		},
	}

	assert.True(t, PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingSucceededCondition(t *testing.T) {
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "SucceededPR",
			Namespace: ns,
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				Conditions: corev1.Conditions{
					{
						Type:   corev1.ConditionSucceeded,
						Status: corev1.ConditionTrue,
						Reason: "Succeeded",
					},
				},
			},
		},
	}

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
