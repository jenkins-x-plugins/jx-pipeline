package processor

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/yaml"
)

var info = termcolor.ColorInfo

// ProcessFile processes the given file with the processor
func ProcessFile(processor Interface, path string) (bool, error) {
	var err error
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return false, errors.Errorf("empty file file %s", path)
	}

	message := fmt.Sprintf("for file %s", path)

	kindPrefix := "kind:"
	kind := "PipelineRun"
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, kindPrefix) {
			continue
		}
		k := strings.TrimSpace(line[len(kindPrefix):])
		if k != "" {
			kind = k
			break
		}
	}
	modified := false
	var resource interface{}

	switch kind {
	case "Pipeline":
		pipeline := &tektonv1beta1.Pipeline{}
		resource = pipeline
		err = yaml.Unmarshal(data, pipeline)
		if err != nil {
			return false, errors.Wrapf(err, "failed to unmarshal Pipeline YAML %s", message)
		}
		modified, err = processor.ProcessPipeline(pipeline, path)

	case "PipelineRun":
		prs := &tektonv1beta1.PipelineRun{}
		resource = prs
		err = yaml.Unmarshal(data, prs)
		if err != nil {
			return false, errors.Wrapf(err, "failed to unmarshal PipelineRun YAML %s", message)
		}
		modified, err = processor.ProcessPipelineRun(prs, path)

	case "Task":
		task := &tektonv1beta1.Task{}
		resource = task
		err = yaml.Unmarshal(data, task)
		if err != nil {
			return false, errors.Wrapf(err, "failed to unmarshal Task YAML %s", message)
		}
		modified, err = processor.ProcessTask(task, path)

	case "TaskRun":
		tr := &tektonv1beta1.TaskRun{}
		resource = tr
		err = yaml.Unmarshal(data, tr)
		if err != nil {
			return false, errors.Wrapf(err, "failed to unmarshal TaskRun YAML %s", message)
		}
		modified, err = processor.ProcessTaskRun(tr, path)

	default:
		log.Logger().Debugf("kind %s is not supported for %s", kind, message)
		return false, nil
	}

	if err != nil {
		return false, errors.Wrapf(err, "failed to process %s", message)
	}
	if !modified {
		return false, nil
	}

	err = yamls.SaveFile(resource, path)
	if err != nil {
		return false, errors.Wrapf(err, "failed to save file %s", path)
	}
	log.Logger().Infof("saved file %s", info(path))
	return modified, nil
}

// ProcessPipelineSpec default function for processing a pipeline spec which may be nil
func ProcessPipelineSpec(ps *tektonv1beta1.PipelineSpec, path string, fn func(ts *tektonv1beta1.TaskSpec, path, name string) (bool, error)) (bool, error) {
	if ps == nil {
		return false, nil
	}
	modified := false
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec == nil {
			continue
		}
		name := pt.Name
		ts := &pt.TaskSpec.TaskSpec

		flag, err := fn(ts, path, name)
		if err != nil {
			return false, errors.Wrapf(err, "failed to process task spec")
		}
		if flag {
			modified = true
		}
	}
	return modified, nil
}
