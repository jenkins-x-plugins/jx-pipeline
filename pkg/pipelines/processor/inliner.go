package processor

import (
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

type inliner struct {
	input            input.Interface
	resolver         *inrepo.UsesResolver
	defaultSHA       string
	step             string
	inlineProperties []string
}

// NewInliner
func NewInliner(input input.Interface, resolver *inrepo.UsesResolver, defaultSHA, step string, inlineProperties []string) *inliner { //nolint:revive
	return &inliner{
		input:            input,
		resolver:         resolver,
		defaultSHA:       defaultSHA,
		step:             step,
		inlineProperties: inlineProperties,
	}
}

func (p *inliner) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, path)
}

func (p *inliner) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, path)
}

func (p *inliner) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, path, task.Name)
}

func (p *inliner) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	return false, nil
}

func (p *inliner) processPipelineSpec(ps *v1beta1.PipelineSpec, path string) (bool, error) {
	return ProcessPipelineSpec(ps, path, p.processTaskSpec)
}

//nolint
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
		image := step.Image
		if image == "" {
			image = templateImage
		}
		uses := strings.TrimPrefix(image, "uses:")
		if uses == image {
			continue
		}
		task := false
		if name == "" {
			name = image
			task = true
		}
		names = append(names, name)
		stepOptions[name] = &stepOption{
			step:  step,
			uses:  uses,
			image: image,
			index: i,
			task:  task,
		}
	}

	var err error
	name = p.step
	if name == "" {
		name, err = p.input.PickNameWithDefault(names, "pick the step: ", "", "select the name of the step to override")
		if err != nil {
			return false, errors.Wrapf(err, "failed to pick step")
		}
	}
	if name == "" {
		return false, errors.Errorf("no step name selected")
	}
	so := stepOptions[name]
	if so == nil {
		return false, errors.Errorf("no step exists for name %s", name)
	}

	step := so.step

	// lets inline the values from the step...
	catalogTaskSpec, err := lighthouses.FindCatalogTaskSpecFromURI(p.resolver, so.uses)
	if err != nil {
		return false, errors.Wrapf(err, "failed to find the pipeline catalog TaskSpec for %s", path)
	}
	if catalogTaskSpec == nil {
		return false, errors.Errorf("could not resolve TaskSpec for uses %s", so.uses)
	}

	if !so.task {
		catalogStep := FindStep(catalogTaskSpec, step.Name)
		if catalogStep == nil {
			return false, errors.Wrapf(err, "could not find step: %s in the catalog", step.Name)
		}

		// lets replace with the catalog step or inline specific properties
		err = p.inlineStep(step, catalogStep)
		if err != nil {
			return false, errors.Wrapf(err, "failed to inline properties")
		}
		return true, nil
	}

	// lets inline all the steps in the uses task
	steps := ts.Steps[0:so.index]
	for k := range catalogTaskSpec.Steps {
		s := catalogTaskSpec.Steps[k]
		newStep := v1beta1.Step{}
		newStep.Name = s.Name
		if ts.StepTemplate == nil {
			ts.StepTemplate = &corev1.Container{}
		}
		if ts.StepTemplate.Image == "" {
			ts.StepTemplate.Image = so.image
		}
		if ts.StepTemplate.Image != so.image {
			newStep.Image = so.image
		}
		steps = append(steps, newStep)
	}
	if so.index+1 < len(ts.Steps) {
		steps = append(steps, ts.Steps[so.index+1:]...)
	}
	ts.Steps = steps

	// now lets pick one of the tasks to inline
	names = nil
	catalogSteps := map[string]*v1beta1.Step{}
	for i := range catalogTaskSpec.Steps {
		s := &catalogTaskSpec.Steps[i]
		n := s.Name
		if n == "" {
			continue
		}
		names = append(names, n)
		catalogSteps[n] = s
	}
	name, err = p.input.PickNameWithDefault(names, "pick the step to inline: ", "", "select the name of the step to override")
	if err != nil {
		return false, errors.Wrapf(err, "failed to pick step")
	}

	if name != "" {
		catalogStep := catalogSteps[name]
		if catalogStep != nil {
			found := false
			for i := range ts.Steps {
				s := &ts.Steps[i]
				if s.Name != name {
					continue
				}
				err = p.inlineStep(s, catalogStep)
				if err != nil {
					return false, errors.Wrapf(err, "failed to inline properties")
				}

				found = true
				break
			}
			if !found {
				return false, errors.Errorf("could not find step %s in resulting task", name)
			}
		}
	}
	return true, nil
}

func (p *inliner) inlineStep(s, catalogStep *v1beta1.Step) error {
	if len(p.inlineProperties) == 0 {
		*s = *catalogStep
		return nil
	}

	for _, prop := range p.inlineProperties {
		switch prop {
		case "args":
			s.Args = catalogStep.Args
		case "onError":
			s.OnError = catalogStep.OnError
		case "command":
			s.Command = catalogStep.Command
		case "env":
			if len(catalogStep.Env) > 0 {
				s.Env = catalogStep.Env
			}
		case "envFrom":
			if len(catalogStep.EnvFrom) > 0 {
				s.EnvFrom = catalogStep.EnvFrom
			}
		case "image":
			s.Image = catalogStep.Image
		case "imagePullPolicy":
			s.ImagePullPolicy = catalogStep.ImagePullPolicy
		case "lifecycle":
			s.Lifecycle = catalogStep.Lifecycle
		case "livenessProbe":
			if catalogStep.LivenessProbe != nil {
				s.LivenessProbe = catalogStep.LivenessProbe
			}
		case "readinessProbe":
			if catalogStep.ReadinessProbe != nil {
				s.ReadinessProbe = catalogStep.ReadinessProbe
			}
		case "resources":
			s.Resources = catalogStep.Resources
		case "script":
			s.Script = catalogStep.Script
		case "securityContext":
			s.SecurityContext = catalogStep.SecurityContext
		case "startupProbe":
			if catalogStep.StartupProbe != nil {
				s.StartupProbe = catalogStep.StartupProbe
			}
		case "terminationMessagePath":
			s.TerminationMessagePath = catalogStep.TerminationMessagePath
		case "terminationMessagePolicy":
			s.TerminationMessagePolicy = catalogStep.TerminationMessagePolicy
		case "timeout":
			if catalogStep.Timeout != nil {
				s.Timeout = catalogStep.Timeout
			}
		case "volumeDevices":
			s.VolumeDevices = catalogStep.VolumeDevices
		case "volumeMounts":
			s.VolumeMounts = catalogStep.VolumeMounts
		case "workspaces":
			if len(catalogStep.Workspaces) > 0 {
				s.Workspaces = catalogStep.Workspaces
			}
		case "workingDir":
			s.WorkingDir = catalogStep.WorkingDir
		default:
			return errors.Errorf("invalid step property: %s", prop)
		}
	}
	return nil
}

type stepOption struct {
	step  *v1beta1.Step
	uses  string
	image string
	index int
	task  bool
}
