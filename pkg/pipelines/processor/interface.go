package processor

import (
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// Interface the interface for a pipeline processor
type Interface interface {
	// ProcessTask processes a Task and returns if its modified and/or error
	ProcessTask(task *tektonv1beta1.Task, path string) (bool, error)

	// ProcessTaskRun processes a TaskRun and returns if its modified and/or error
	ProcessTaskRun(tr *tektonv1beta1.TaskRun, path string) (bool, error)

	// ProcessPipeline processes a Pipeline and returns if its modified and/or error
	ProcessPipeline(pipeline *tektonv1beta1.Pipeline, path string) (bool, error)

	// ProcessPipelineRun processes a PipelineRun and returns if its modified and/or error
	ProcessPipelineRun(prs *tektonv1beta1.PipelineRun, path string) (bool, error)
}
