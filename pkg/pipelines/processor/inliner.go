package processor

import (
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type inliner struct {
	input      input.Interface
	resolver   *inrepo.UsesResolver
	defaultSHA string
}

// NewInliner
func NewInliner(input input.Interface, resolver *inrepo.UsesResolver, defaultSHA string) *inliner {
	return &inliner{
		input:      input,
		resolver:   resolver,
		defaultSHA: defaultSHA,
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
	templateImage := ""
	if ts.StepTemplate != nil {
		templateImage = ts.StepTemplate.Image
	}
	stepOptions := map[string]*stepOption{}
	var names []string
	for i := range ts.Steps {
		step := &ts.Steps[i]
		name := step.Name
		if name == "" {
			continue
		}
		image := step.Image
		if image == "" {
			image = templateImage
		}
		uses := strings.TrimPrefix(image, "uses:")
		if uses == image {
			continue
		}

		names = append(names, name)
		stepOptions[name] = &stepOption{
			step:  step,
			uses:  image,
			index: i,
		}
	}

	var err error
	name, err = p.input.PickNameWithDefault(names, "pick the step: ", "", "select the name of the step to override")
	if err != nil {
		return false, errors.Wrapf(err, "failed to pick step")
	}
	if name == "" {
		return false, errors.Errorf("no step name selected")
	}
	so := stepOptions[name]
	if so == nil {
		return false, errors.Errorf("no step exists for name %s", name)
	}

	// lets inline the values from the step...
	catalogTaskSpec, err := lighthouses.FindCatalogTaskSpec(p.resolver, path, p.defaultSHA)
	if err != nil {
		return false, errors.Wrapf(err, "failed to find the pipeline catalog TaskSpec for %s", path)
	}
	step := so.step
	catalogStep := FindStep(catalogTaskSpec, step.Name)
	if catalogStep == nil {
		return false, errors.Wrapf(err, "could not find step: %s in the catalog", step.Name)
	}

	// lets replace with the catalog step
	// TODO longer term we could ask users to pick which things to override
	*step = *catalogStep
	return true, nil
}

type stepOption struct {
	step  *v1beta1.Step
	uses  string
	index int
}
