package processor

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type inliner struct {
	catalogTaskSpec *v1beta1.TaskSpec
	input           input.Interface
}

// NewInliner
func NewInliner(input input.Interface, catalogTaskSpec *v1beta1.TaskSpec) *inliner {
	return &inliner{
		catalogTaskSpec: catalogTaskSpec,
		input:           input,
	}
}

func (p *inliner) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, &pipeline.ObjectMeta, path, pipeline)
}

func (p *inliner) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, &prs.ObjectMeta, path, prs)
}

func (p *inliner) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, path, task.Name)
}

func (p *inliner) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	// TODO
	return false, nil
}

func (p *inliner) processPipelineSpec(ps *v1beta1.PipelineSpec, metadata *metav1.ObjectMeta, path string, resource interface{}) (bool, error) {
	return ProcessPipelineSpec(ps, path, p.processTaskSpec)
}

func (p *inliner) processTaskSpec(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
	modified := false

	for i := range ts.Steps {
		step := &ts.Steps[i]
		image := step.Image
		uses := strings.TrimPrefix(image, "uses:")
		if uses == image {
			continue
		}

		catalogStep := FindStep(p.catalogTaskSpec, step.Name)
		if catalogStep == nil {
			// this step is not in the catalog so don' replace with uses:
			continue
		}

	}
	return modified, nil
}
