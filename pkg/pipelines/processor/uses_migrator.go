package processor

import (
	"fmt"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"strings"
)

type usesMigrator struct {
	dir         string
	owner       string
	repository  string
	tasksFolder string
}

// NewUsesMigrator creates a new uses migrator
func NewUsesMigrator(dir, tasksFolder, owner, repository string) Interface {
	return &usesMigrator{
		dir:         dir,
		tasksFolder: tasksFolder,
		owner:       owner,
		repository:  repository,
	}
}

func (p *usesMigrator) ProcessPipeline(pipeline *v1beta1.Pipeline, path string) (bool, error) {
	return p.processPipelineSpec(&pipeline.Spec, path, pipeline)
}

func (p *usesMigrator) ProcessPipelineRun(prs *v1beta1.PipelineRun, path string) (bool, error) {
	return p.processPipelineSpec(prs.Spec.PipelineSpec, path, prs)
}

func (p *usesMigrator) ProcessTask(task *v1beta1.Task, path string) (bool, error) {
	return p.processTaskSpec(&task.Spec, path, task.Name)
}

func (p *usesMigrator) ProcessTaskRun(tr *v1beta1.TaskRun, path string) (bool, error) {
	// TODO
	return false, nil
}

func (p *usesMigrator) processPipelineSpec(ps *v1beta1.PipelineSpec, path string, resource interface{}) (bool, error) {
	hasRealImage, err := ProcessPipelineSpec(ps, path, hasRealImage)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check for real times")
	}
	if !hasRealImage {
		return false, nil
	}
	err = p.saveOriginalResource(path, resource)
	if err != nil {
		return false, errors.Wrapf(err, "failed to save original resource so we can reuse")
	}
	return ProcessPipelineSpec(ps, path, p.processTaskSpec)
}

func (p *usesMigrator) processTaskSpec(ts *v1beta1.TaskSpec, path, name string) (bool, error) {
	usesPath, err := p.usesPath(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get uses: path")
	}
	if usesPath == "" {
		return false, nil
	}

	modified := false
	for i := range ts.Steps {
		step := &ts.Steps[i]
		image := step.Image
		uses := strings.TrimPrefix(image, "uses:")
		if uses != image {
			continue
		}
		usesImage := fmt.Sprintf("uses:%s/%s/%s@head", p.owner, p.repository, usesPath)
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
