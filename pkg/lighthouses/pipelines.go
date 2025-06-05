package lighthouses

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// LoadEffectivePipelineRun loads the effective pipeline run
func LoadEffectivePipelineRun(resolver *inrepo.UsesResolver, path string) (*pipelinev1.PipelineRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file: %s", path)
	}

	dir := filepath.Dir(path)
	resolver.Dir = dir
	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML file %s: %w", path, err)
	}
	return pr, nil
}
