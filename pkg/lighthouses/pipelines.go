package lighthouses

import (
	"io/ioutil"
	"path/filepath"

	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// LoadEffectivePipelineRun loads the effective pipeline run
func LoadEffectivePipelineRun(resolver *inrepo.UsesResolver, path string) (*tektonv1beta1.PipelineRun, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load file %s", path)
	}
	if len(data) == 0 {
		return nil, errors.Errorf("empty file file %s", path)
	}

	dir := filepath.Dir(path)
	resolver.Dir = dir
	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal YAML file %s", path)
	}
	return pr, nil
}
