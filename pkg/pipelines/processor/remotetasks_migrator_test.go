package processor_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines/processor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	baseTask = &pipelinev1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-task",
		},
		Spec: pipelinev1.TaskSpec{
			StepTemplate: &pipelinev1.StepTemplate{},
			Steps: []pipelinev1.Step{
				{Image: "test-image"},
			},
		},
	}

	baseExpectedTask = &pipelinev1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-task",
		},
		Spec: pipelinev1.TaskSpec{
			Params: processor.LighthouseTaskParams,
			StepTemplate: &pipelinev1.StepTemplate{
				Env:     append(processor.LighthouseEnvs, processor.HomeEnv),
				EnvFrom: processor.DefaultEnvFroms,
			},
			Steps: []pipelinev1.Step{
				{Image: "test-image"},
			},
		},
	}
)

func TestRemoteTasksMigrator_ProcessTask(t *testing.T) {
	testCases := []struct {
		name                 string
		baseTaskModifier     func(task *pipelinev1.Task)
		expectedTaskModifier func(task *pipelinev1.Task)
	}{
		{
			name:                 "BaseTask",
			baseTaskModifier:     func(_ *pipelinev1.Task) {},
			expectedTaskModifier: func(_ *pipelinev1.Task) {},
		},
		{
			name: "WithExistingUnrelatedEnvs",
			baseTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = []v1.EnvVar{
					{Name: "UNRELATED_ENV", Value: "unrelated-value"},
				}
			},
			expectedTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = append([]v1.EnvVar{
					{Name: "UNRELATED_ENV", Value: "unrelated-value"},
				}, task.Spec.StepTemplate.Env...)
			},
		},
		{
			name: "WithExistingRelatedEnvs",
			baseTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = []v1.EnvVar{
					{Name: "BUILD_ID", Value: "$(params.BUILD_ID)"},
				}
			},
			expectedTaskModifier: func(_ *pipelinev1.Task) {},
		},
		{
			name: "WithExistingIncorrectHomeEnv",
			baseTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = []v1.EnvVar{
					{Name: "HOME", Value: "/tekton/workspace"},
				}
			},
			expectedTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = append([]v1.EnvVar{
					{Name: "HOME", Value: "/workspace"},
				}, processor.LighthouseEnvs...)
			},
		},
		{
			name: "WithExistingIncorrectHomeEnv",
			baseTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = []v1.EnvVar{
					{Name: "HOME", Value: "/tekton/workspace"},
				}
			},
			expectedTaskModifier: func(task *pipelinev1.Task) {
				task.Spec.StepTemplate.Env = append([]v1.EnvVar{
					{Name: "HOME", Value: "/workspace"},
				}, processor.LighthouseEnvs...)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inputTask := baseTask.DeepCopy()
			tc.baseTaskModifier(inputTask)

			migrator := processor.RemoteTasksMigrator{}
			isProcessed, err := migrator.ProcessTask(inputTask, "/some/path")
			assert.NoError(t, err)
			assert.True(t, isProcessed)

			expectedTask := baseExpectedTask.DeepCopy()
			tc.expectedTaskModifier(expectedTask)

			assert.Equal(t, expectedTask, inputTask)
		})
	}
}

func TestRemoteTasksMigrator_ProcessPipelineRun_ParentPipelineRun(t *testing.T) {
	// Parent PipelineRuns should be processed into multiple tasks
	migrator := processor.RemoteTasksMigrator{}
	prsPath := "./remotetasks_data/parent_pipelinerun.yaml"

	var prs *pipelinev1.PipelineRun
	err := yamls.LoadFile(prsPath, &prs)
	assert.NoError(t, err)

	originalSteps := prs.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps

	isModified, err := migrator.ProcessPipelineRun(prs, prsPath)
	assert.NoError(t, err)
	assert.False(t, isModified)

	tasksDir := filepath.Join(filepath.Dir(prsPath), prs.Name)
	assert.DirExists(t, tasksDir)
	defer os.RemoveAll(tasksDir)

	entries, err := os.ReadDir(tasksDir)
	assert.NoError(t, err)
	assert.Equal(t, len(originalSteps), len(entries))

	for _, entry := range entries {
		actualTaskPath := filepath.Join(tasksDir, entry.Name())
		var actualTask *pipelinev1.Task
		err := yamls.LoadFile(actualTaskPath, &actualTask)
		assert.NoError(t, err)

		expectedTaskPath := fmt.Sprint("./remotetasks_data/parent_pipelinerun_expected/", entry.Name())
		var expectedTask *pipelinev1.Task
		err = yamls.LoadFile(expectedTaskPath, &expectedTask)
		assert.NoError(t, err)

		assert.Equal(t, expectedTask, actualTask)
	}
}

func TestRemoteTasksMigrator_ProcessPipelineRun_ChildPipelineRun(t *testing.T) {
	// Child PipelineRuns should be processed into a new pipelineRun
	migrator := processor.NewRemoteTasksMigrator("", resource.MustParse("1Gi"))
	inputPRSPath := "./remotetasks_data/child_pipelinerun.yaml"
	expectedPRSPath := "./remotetasks_data/child_pipelinerun_expected.yaml"

	var prs *pipelinev1.PipelineRun
	err := yamls.LoadFile(inputPRSPath, &prs)
	assert.NoError(t, err)

	newPrsPath := filepath.Join(filepath.Dir(inputPRSPath), prs.Name+".yaml")
	isModified, err := migrator.ProcessPipelineRun(prs, newPrsPath)
	assert.NoError(t, err)
	assert.True(t, isModified)

	var expected *pipelinev1.PipelineRun
	err = yamls.LoadFile(expectedPRSPath, &expected)
	assert.NoError(t, err)

	assert.Equal(t, expected, prs)
}
