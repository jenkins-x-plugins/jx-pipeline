package processor

import pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

// Interface the interface for a pipeline processor
type Interface interface {
	// ProcessTask processes a Task and returns if its modified and/or error
	ProcessTask(task *pipelinev1.Task, path string) (bool, error)

	// ProcessTaskRun processes a TaskRun and returns if its modified and/or error
	ProcessTaskRun(tr *pipelinev1.TaskRun, path string) (bool, error)

	// ProcessPipeline processes a Pipeline and returns if its modified and/or error
	ProcessPipeline(pipeline *pipelinev1.Pipeline, path string) (bool, error)

	// ProcessPipelineRun processes a PipelineRun and returns if its modified and/or error
	ProcessPipelineRun(prs *pipelinev1.PipelineRun, path string) (bool, error)
}
