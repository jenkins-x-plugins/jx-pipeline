package processor

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type modifier struct {
	templateEnvs map[string]string
}

// NewModifier
func NewModifier(templateEnvs map[string]string) *modifier {
	return &modifier{
		templateEnvs: templateEnvs,
	}
}

func (p *modifier) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, &pipeline.ObjectMeta, path, pipeline)
}

func (p *modifier) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, &prs.ObjectMeta, path, prs)
}

func (p *modifier) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, path, task.Name)
}

func (p *modifier) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	return false, nil
}

func (p *modifier) processPipelineSpec(ps *v1beta1.PipelineSpec, metadata *metav1.ObjectMeta, path string, resource interface{}) (bool, error) {
	return ProcessPipelineSpec(ps, path, p.processTaskSpec)
}

func (p *modifier) processTaskSpec(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
	if ts.StepTemplate == nil {
		ts.StepTemplate = &corev1.Container{}
	}
	modified := false
	if p.templateEnvs != nil {
		for k, v := range p.templateEnvs {
			found := false
			for i := range ts.StepTemplate.Env {
				env := &ts.StepTemplate.Env[i]
				if env.Name == k {
					found = true
					if env.Value != v {
						env.Value = v
						modified = true
					}
					break
				}
			}
			if !found {
				ts.StepTemplate.Env = append(ts.StepTemplate.Env, corev1.EnvVar{
					Name:  k,
					Value: v,
				})
				modified = true
			}
		}
	}
	return modified, nil
}
