package processor

import (
	"fmt"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type usesMigrator struct {
	dir         string
	owner       string
	repository  string
	tasksFolder string
	concise     bool
}

// NewUsesMigrator creates a new uses migrator
func NewUsesMigrator(dir, tasksFolder, owner, repository string) Interface {
	return &usesMigrator{
		dir:         dir,
		tasksFolder: tasksFolder,
		owner:       owner,
		repository:  repository,
		concise:     true,
	}
}

func (p *usesMigrator) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, &pipeline.ObjectMeta, path, pipeline)
}

func (p *usesMigrator) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, &prs.ObjectMeta, path, prs)
}

func (p *usesMigrator) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, &task.ObjectMeta, path, task.Name)
}

func (p *usesMigrator) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	// TODO
	return false, nil
}

func (p *usesMigrator) processPipelineSpec(ps *v1beta1.PipelineSpec, metadata *metav1.ObjectMeta, path string, resource interface{}) (bool, error) {
	hasRealImage, err := ProcessPipelineSpec(ps, path, hasRealImage)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for real times")
	}
	if !hasRealImage {
		return false, nil
	}

	// lets remove the old annotations
	if metadata.Annotations != nil {
		delete(metadata.Annotations, inrepo.AppendStepURL)
		delete(metadata.Annotations, inrepo.PrependStepURL)
	}
	err = p.saveOriginalResource(path, resource)
	if err != nil {
		return false, errors.Wrapf(err, "failed to save original resource so we can reuse")
	}
	fn := func(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
		return p.processTaskSpec(ts, metadata, path, name)
	}
	return ProcessPipelineSpec(ps, path, fn)
}

func (p *usesMigrator) processTaskSpec(ts *v1beta1.TaskSpec, metadata *metav1.ObjectMeta, path, name string) (bool, error) {
	usesPath, err := p.usesPath(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get uses: path")
	}
	if usesPath == "" {
		return false, nil
	}

	ann := metadata.Annotations
	if ann == nil {
		ann = map[string]string{}
	}

	modified := replaceStepAnnotations(ann, ts)
	for i := range ts.Steps {
		step := &ts.Steps[i]
		image := step.Image
		uses := strings.TrimPrefix(image, "uses:")
		if uses != image {
			continue
		}
		usesImage := fmt.Sprintf("uses:%s/%s/%s@versionStream", p.owner, p.repository, usesPath)

		if ts.StepTemplate == nil {
			ts.StepTemplate = &corev1.Container{}
		}
		if ts.StepTemplate.Image == "" {
			ts.StepTemplate.Image = usesImage
		}
		if usesImage == ts.StepTemplate.Image {
			usesImage = ""
		}

		// lets translate to the uses string
		ts.Steps[i] = v1beta1.Step{
			Container: corev1.Container{
				Name:  step.Name,
				Image: usesImage,
			},
		}
		modified = true
	}
	return modified, nil
}

func replaceStepAnnotations(ann map[string]string, ts *v1beta1.TaskSpec) bool {
	modified := false
	value := ConvertLegacyStepAnnotationURLToUsesImage(ann, inrepo.PrependStepURL)
	if value != "" {
		modified = true
		newSteps := []v1beta1.Step{
			{
				Container: corev1.Container{
					Image: value,
				},
			},
		}
		ts.Steps = append(newSteps, ts.Steps...)
	}
	value = ConvertLegacyStepAnnotationURLToUsesImage(ann, inrepo.AppendStepURL)
	if value != "" {
		modified = true
		ts.Steps = append(ts.Steps, v1beta1.Step{
			Container: corev1.Container{
				Image: value,
			},
		})
	}
	return modified
}

// ConvertLegacyStepAnnotationURLToUsesImage converts the given append annotation URL to a uses string if its not blank
func ConvertLegacyStepAnnotationURLToUsesImage(ann map[string]string, key string) string {
	text := ann[key]
	if text == "" {
		return ""
	}
	delete(ann, key)
	u, err := url.Parse(text)
	if err == nil {
		// lets try convert to a nice git URI
		if u.Host == "raw.githubusercontent.com" {
			paths := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
			if len(paths) > 1 {
				path := ""
				sha := "versionStream"
				if len(paths) > 2 {
					sha = paths[2]
					if len(paths) > 3 {
						path = strings.Join(paths[3:], "/")
					}
				}
				gu := &inrepo.GitURI{
					Owner:      paths[0],
					Repository: paths[1],
					Path:       path,
					SHA:        sha,
				}
				if gu.Owner == "jenkins-x" && gu.Repository == "jx3-pipeline-catalog" {
					gu.SHA = "versionStream"
				}
				return "uses:" + gu.String()
			}
		}
	}
	return "uses:" + text
}

func (p *usesMigrator) usesPath(path string) (string, error) {
	// lets make sure we save the original file
	rel, err := filepath.Rel(p.dir, path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find relative path to %s for %s", p.dir, path)
	}

	// lets save the raw image in the tasks folder
	paths := strings.Split(rel, string(os.PathSeparator))
	if len(paths) < 3 || paths[0] != "packs" {
		// lets ignore this file
		return "", nil
	}

	return filepath.Join(p.tasksFolder, paths[1], paths[len(paths)-1]), nil
}

// saveOriginalResource lets copy the original to the tasks folder so we can then use it
func (p *usesMigrator) saveOriginalResource(path string, resource interface{}) error {
	// lets make sure we save the original file
	rel, err := filepath.Rel(p.dir, path)
	if err != nil {
		return errors.Wrapf(err, "failed to find relative path to %s for %s", p.dir, path)
	}

	// lets save the raw image in the tasks folder
	paths := strings.Split(rel, string(os.PathSeparator))
	if len(paths) < 3 || paths[0] != "packs" {
		// lets ignore this file
		return nil
	}

	outDir := filepath.Join(p.dir, p.tasksFolder, paths[1])
	err = os.MkdirAll(outDir, files.DefaultDirWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to make dir %s", outDir)
	}
	outFile := filepath.Join(outDir, paths[len(paths)-1])

	err = yamls.SaveFile(resource, outFile)
	if err != nil {
		return errors.Wrapf(err, "failed to save file %s", outFile)
	}
	log.Logger().Infof("saved reuse file %s", info(outFile))
	return nil
}

func hasRealImage(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
	hasRealImage := false
	for i := range ts.Steps {
		step := &ts.Steps[i]
		if !strings.HasPrefix(step.Image, "uses:") {
			hasRealImage = true
		}
	}
	return hasRealImage, nil
}
