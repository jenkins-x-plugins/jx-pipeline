//go:build unit
// +build unit

package tektonlog_test

import (
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/stretchr/testify/assert"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ns = "jx"
)

func TestPipelineRunIsNotPendingCompletedRun(t *testing.T) {
	now := metav1.Now()
	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PR1",
			Namespace: ns,
			Labels: map[string]string{
				tektonlog.LabelRepo:    "fakerepo",
				tektonlog.LabelBranch:  "fakebranch",
				tektonlog.LabelOwner:   "fakeowner",
				tektonlog.LabelContext: "fakecontext",
			},
		},
		Spec: pipelinev1.PipelineRunSpec{
			Params: []pipelinev1.Param{
				{
					Name: "version",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "v1",
					},
				},
				{
					Name: "build_id",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				CompletionTime: &now,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingRunningSteps(t *testing.T) {
	taskRunStatusMap := make(map[string]*pipelinev1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &pipelinev1.PipelineRunTaskRunStatus{
		Status: &pipelinev1.TaskRunStatus{
			TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
				Steps: []pipelinev1.StepState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				}},
			},
		},
	}

	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PR1",
			Namespace: ns,
			Labels: map[string]string{
				tektonlog.LabelRepo:    "fakerepo",
				tektonlog.LabelBranch:  "fakebranch",
				tektonlog.LabelOwner:   "fakeowner",
				tektonlog.LabelContext: "fakecontext",
			},
		},
		Spec: pipelinev1.PipelineRunSpec{
			Params: []pipelinev1.Param{
				{
					Name: "version",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingWaitingSteps(t *testing.T) {
	taskRunStatusMap := make(map[string]*pipelinev1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &pipelinev1.PipelineRunTaskRunStatus{
		Status: &pipelinev1.TaskRunStatus{
			TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
				Steps: []pipelinev1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Message: "Pending",
						},
					},
				}},
			},
		},
	}

	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PR1",
			Namespace: ns,
			Labels: map[string]string{
				tektonlog.LabelRepo:    "fakerepo",
				tektonlog.LabelBranch:  "fakebranch",
				tektonlog.LabelOwner:   "fakeowner",
				tektonlog.LabelContext: "fakecontext",
			},
		},
		Spec: pipelinev1.PipelineRunSpec{
			Params: []pipelinev1.Param{
				{
					Name: "version",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.False(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingWaitingStepsInPodInitializing(t *testing.T) {
	taskRunStatusMap := make(map[string]*pipelinev1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &pipelinev1.PipelineRunTaskRunStatus{
		Status: &pipelinev1.TaskRunStatus{
			TaskRunStatusFields: pipelinev1.TaskRunStatusFields{
				Steps: []pipelinev1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "PodInitializing",
						},
					},
				}},
			},
		},
	}

	pr := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PR1",
			Namespace: ns,
			Labels: map[string]string{
				tektonlog.LabelRepo:    "fakerepo",
				tektonlog.LabelBranch:  "fakebranch",
				tektonlog.LabelOwner:   "fakeowner",
				tektonlog.LabelContext: "fakecontext",
			},
		},
		Spec: pipelinev1.PipelineRunSpec{
			Params: []pipelinev1.Param{
				{
					Name: "version",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: pipelinev1.ArrayOrString{
						Type:      pipelinev1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: pipelinev1.PipelineRunStatus{
			PipelineRunStatusFields: pipelinev1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}
