package tektonlog

import (
	"context"
	"fmt"

	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineType is used to differentiate between actual build pipelines and pipelines to create the build pipelines,
// aka meta pipelines.
type PipelineType int

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
func CancelPipelineRun(ctx context.Context, tektonClient tektonclient.Interface, ns string, pr *pipelineapi.PipelineRun) error {
	prName := pr.Name
	var err error
	pr, err = tektonClient.TektonV1().PipelineRuns(ns).Get(ctx, prName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get PipelineRun %s in namespace %s: %w", prName, ns, err)
	}
	pr.Spec.Status = pipelineapi.PipelineRunSpecStatusCancelled
	_, err = tektonClient.TektonV1().PipelineRuns(ns).Update(ctx, pr, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update PipelineRun %s in namespace %s to mark it as cancelled: %w", prName, ns, err)
	}
	return nil
}
