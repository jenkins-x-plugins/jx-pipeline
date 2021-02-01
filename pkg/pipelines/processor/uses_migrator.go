package processor

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UsesMigrator struct {
	CatalogTaskSpec *v1beta1.TaskSpec
	Dir             string
	Owner           string
	Repository      string
	TasksFolder     string
	SHA             string
	catalog         bool
	concise         bool
}

// NewUsesMigrator creates a new uses migrator
func NewUsesMigrator(dir, tasksFolder, owner, repository string, catalog bool) *UsesMigrator {
	return &UsesMigrator{
		Dir:         dir,
		TasksFolder: tasksFolder,
		Owner:       owner,
		Repository:  repository,
		catalog:     catalog,
		concise:     true,
	}
}

func (p *UsesMigrator) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, &pipeline.ObjectMeta, path, pipeline)
}

func (p *UsesMigrator) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, &prs.ObjectMeta, path, prs)
}

func (p *UsesMigrator) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, &task.ObjectMeta, path, task.Name)
}

func (p *UsesMigrator) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	// TODO
	return false, nil
}

func (p *UsesMigrator) processPipelineSpec(ps *v1beta1.PipelineSpec, metadata *metav1.ObjectMeta, path string, resource interface{}) (bool, error) {
	hasRealImage, err := ProcessPipelineSpec(ps, path, hasRealImage)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for real times")
	}
	if !hasRealImage {
		return false, nil
	}

	if p.catalog {
		originalMetadata := *metadata
		originalMetadata.Annotations = map[string]string{}

		// lets remove the old annotations
		if metadata.Annotations != nil {
			for k, v := range metadata.Annotations {
				originalMetadata.Annotations[k] = v
			}
			delete(metadata.Annotations, inrepo.AppendStepURL)
			delete(metadata.Annotations, inrepo.PrependStepURL)
		}
		err = p.saveOriginalResource(path, resource)
		if err != nil {
			return false, errors.Wrapf(err, "failed to save original resource so we can reuse")
		}

		// lets use the original metadata for the migration of prepend/append steps
		metadata = &originalMetadata
	}
	fn := func(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
		return p.processTaskSpec(ts, metadata, path, name)
	}
	return ProcessPipelineSpec(ps, path, fn)
}

func (p *UsesMigrator) processTaskSpec(ts *v1beta1.TaskSpec, metadata *metav1.ObjectMeta, path, name string) (bool, error) {
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
		var catalogStep *v1beta1.Step
		if !p.catalog {
			catalogStep = FindStep(p.CatalogTaskSpec, step.Name)
			if catalogStep == nil {
				// this step is not in the catalog so don' replace with uses:
				continue
			}

			// lets not reuse a step if the images are different (other than version)
			if ImageWithoutVersionTag(image) != ImageWithoutVersionTag(catalogStep.Image) {
				continue
			}
		}
		sha := p.SHA
		if sha == "" {
			sha = "versionStream"
		}
		usesImage := fmt.Sprintf("uses:%s/%s/%s@%s", p.Owner, p.Repository, usesPath, sha)

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
		replaceStep := v1beta1.Step{
			Container: corev1.Container{
				Name:  step.Name,
				Image: usesImage,
			},
		}

		if !p.catalog {
			err = p.addLocalOverrides(replaceStep, step, catalogStep)
		}
		ts.Steps[i] = replaceStep
		modified = true
	}
	return modified, nil
}

// ImageWithoutVersionTag returns the image string without any version tag.
func ImageWithoutVersionTag(image string) string {
	idx := strings.LastIndex(image, ":")
	if idx < 0 {
		return image
	}
	return image[0:idx]
}

// FindStep returns the named step or nil
func FindStep(spec *v1beta1.TaskSpec, name string) *v1beta1.Step {
	if spec == nil {
		return nil
	}
	for i := range spec.Steps {
		step := &spec.Steps[i]
		if step.Name == name {
			return step
		}
	}
	return nil
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

func (p *UsesMigrator) usesPath(path string) (string, error) {
	// lets make sure we save the original file
	rel, err := filepath.Rel(p.Dir, path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find relative path to %s for %s", p.Dir, path)
	}
	paths := strings.Split(rel, string(os.PathSeparator))
	if p.catalog {
		// lets save the raw image in the tasks folder
		if len(paths) < 3 || paths[0] != "packs" {
			// lets ignore this file
			return "", nil
		}

		return filepath.Join(p.TasksFolder, paths[1], paths[len(paths)-1]), nil
	}
	return filepath.Join(p.TasksFolder, paths[len(paths)-1]), nil
}

// saveOriginalResource lets copy the original to the tasks folder so we can then use it
func (p *UsesMigrator) saveOriginalResource(path string, resource interface{}) error {
	// lets make sure we save the original file
	rel, err := filepath.Rel(p.Dir, path)
	if err != nil {
		return errors.Wrapf(err, "failed to find relative path to %s for %s", p.Dir, path)
	}

	// lets save the raw image in the tasks folder
	paths := strings.Split(rel, string(os.PathSeparator))
	if len(paths) < 3 || paths[0] != "packs" {
		// lets ignore this file
		return nil
	}

	outDir := filepath.Join(p.Dir, p.TasksFolder, paths[1])
	err = os.MkdirAll(outDir, files.DefaultDirWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to make Dir %s", outDir)
	}
	outFile := filepath.Join(outDir, paths[len(paths)-1])

	err = yamls.SaveFile(resource, outFile)
	if err != nil {
		return errors.Wrapf(err, "failed to save file %s", outFile)
	}
	log.Logger().Infof("saved reuse file %s", info(outFile))
	return nil
}

// addLocalOverrides lets compare the step in the current pipeline catalog to the local step and any differences lets
// keep in the result step
func (p *UsesMigrator) addLocalOverrides(resultStep v1beta1.Step, localStep *v1beta1.Step, catalogStep *v1beta1.Step) error {
	if localStep.Script != catalogStep.Script {
		resultStep.Script = localStep.Script
	}
	if !stringhelpers.StringArraysEqual(localStep.Command, catalogStep.Command) {
		resultStep.Command = localStep.Command
	}
	if !stringhelpers.StringArraysEqual(localStep.Args, catalogStep.Args) {
		resultStep.Args = localStep.Args
	}
	resultStep.Env = overrideEnv(localStep.Env, catalogStep.Env)
	resultStep.EnvFrom = overrideEnvFrom(localStep.EnvFrom, catalogStep.EnvFrom)
	resultStep.VolumeMounts = overrideVolumeMounts(localStep.VolumeMounts, catalogStep.VolumeMounts)
	return nil
}

// overrideEnv returns any locally defined env vars that differ or don't exist in the catalog
func overrideEnv(overrides []corev1.EnvVar, from []corev1.EnvVar) []corev1.EnvVar {
	var answer []corev1.EnvVar
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if f.Name == override.Name {
				if reflect.DeepEqual(f, override) {
					found = true
				}
				break
			}
		}
		if !found {
			answer = append(answer, override)
		}
	}
	return answer
}

// overrideEnvFrom returns any locally defined env froms that differ or don't exist in the catalog
func overrideEnvFrom(overrides []corev1.EnvFromSource, from []corev1.EnvFromSource) []corev1.EnvFromSource {
	var answer []corev1.EnvFromSource
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if reflect.DeepEqual(f, override) {
				found = true
				break
			}
		}
		if !found {
			answer = append(answer, override)
		}
	}
	return answer
}

// overrideVolumeMounts returns any locally defined volume mounts that differ or don't exist in the catalog
func overrideVolumeMounts(overrides []corev1.VolumeMount, from []corev1.VolumeMount) []corev1.VolumeMount {
	var answer []corev1.VolumeMount
	for _, override := range overrides {
		found := false
		for i := range from {
			f := &from[i]
			if reflect.DeepEqual(f, override) {
				found = true
				break
			}
		}
		if !found {
			answer = append(answer, override)
		}
	}
	return answer
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
