package tektonlog

import (
	"github.com/pkg/errors"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
)

// PipelineType is used to differentiate between actual build pipelines and pipelines to create the build pipelines,
// aka meta pipelines.
type PipelineType int

const (
	// BuildPipeline is the yype for the actual build pipeline
	BuildPipeline PipelineType = iota

	// MetaPipeline type for the meta pipeline used to generate the build pipeline
	MetaPipeline
)

func (s PipelineType) String() string {
	return [...]string{"build", "meta"}[s]
}

// PipelineRunIsNotPending returns true if the PipelineRun has completed or has running steps.
func PipelineRunIsNotPending(pr *pipelineapi.PipelineRun) bool {
	if pr.Status.CompletionTime != nil {
		return true
	}
	if len(pr.Status.TaskRuns) > 0 {
		for _, v := range pr.Status.TaskRuns {
			if v.Status != nil {
				for _, stepState := range v.Status.Steps {
					if stepState.Waiting == nil || stepState.Waiting.Reason == "PodInitializing" {
						return true
					}
				}
			}
		}
	}
	return false
}

// PipelineRunIsComplete returns true if the PipelineRun has completed or has running steps.
func PipelineRunIsComplete(pr *pipelineapi.PipelineRun) bool {
	return pr.Status.CompletionTime != nil
}

// CancelPipelineRun cancels a Pipeline
func CancelPipelineRun(tektonClient tektonclient.Interface, ns string, pr *pipelineapi.PipelineRun) error {
	pr.Spec.Status = pipelineapi.PipelineRunSpecStatusCancelled
	_, err := tektonClient.TektonV1beta1().PipelineRuns(ns).Update(pr)
	if err != nil {
		return errors.Wrapf(err, "failed to update PipelineRun %s in namespace %s to mark it as cancelled", pr.Name, ns)
	}
	return nil
}
