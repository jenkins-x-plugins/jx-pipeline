// +build unit

package tektonlog_test

import (
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/stretchr/testify/assert"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ns = "jx"
)

func TestPipelineRunIsNotPendingCompletedRun(t *testing.T) {
	now := metav1.Now()
	pr := &v1beta1.PipelineRun{
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
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name: "version",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "v1",
					},
				},
				{
					Name: "build_id",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
				CompletionTime: &now,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingRunningSteps(t *testing.T) {
	taskRunStatusMap := make(map[string]*v1beta1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &v1beta1.PipelineRunTaskRunStatus{
		Status: &v1beta1.TaskRunStatus{
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				}},
			},
		},
	}

	pr := &v1beta1.PipelineRun{
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
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name: "version",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingWaitingSteps(t *testing.T) {
	taskRunStatusMap := make(map[string]*v1beta1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &v1beta1.PipelineRunTaskRunStatus{
		Status: &v1beta1.TaskRunStatus{
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Message: "Pending",
						},
					},
				}},
			},
		},
	}

	pr := &v1beta1.PipelineRun{
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
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name: "version",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.False(t, tektonlog.PipelineRunIsNotPending(pr))
}

func TestPipelineRunIsNotPendingWaitingStepsInPodInitializing(t *testing.T) {
	taskRunStatusMap := make(map[string]*v1beta1.PipelineRunTaskRunStatus)
	taskRunStatusMap["faketaskrun"] = &v1beta1.PipelineRunTaskRunStatus{
		Status: &v1beta1.TaskRunStatus{
			TaskRunStatusFields: v1beta1.TaskRunStatusFields{
				Steps: []v1beta1.StepState{{
					ContainerState: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "PodInitializing",
						},
					},
				}},
			},
		},
	}

	pr := &v1beta1.PipelineRun{
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
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name: "version",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "v1",
					}},
				{
					Name: "build_id",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			PipelineRunStatusFields: v1beta1.PipelineRunStatusFields{
				TaskRuns: taskRunStatusMap,
			},
		},
	}

	assert.True(t, tektonlog.PipelineRunIsNotPending(pr))
}
