package lighthouses

import (
	"path/filepath"

	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// FindCatalogTaskSpec finds the pipeline catalog TaskSpec
func FindCatalogTaskSpec(resolver *inrepo.UsesResolver, sourceFile string, defaultSHA string) (*v1beta1.TaskSpec, error) {
	owner := resolver.OwnerName
	repo := resolver.RepoName
	sha, err := getCatalogSHA(owner, repo, defaultSHA)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find SHA for catalog repository %s/%s", owner, repo)
	}

	kptPath := filepath.Join(resolver.Dir, sourceFile)

	gu := &inrepo.GitURI{
		Owner:      owner,
		Repository: repo,
		Path:       kptPath,
		SHA:        sha,
	}
	gitURI := gu.String()
	data, err := resolver.GetData(gitURI, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load %s", gitURI)
	}

	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal catalog YAML file %s", kptPath)
	}
	catalogTaskSpec, err := GetMandatoryTaskSpec(pr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find catalog task at %s", gitURI)
	}
	return catalogTaskSpec, nil
}

// getCatalogSHA gets the default SHA
func getCatalogSHA(owner string, repo string, defaultSHA string) (string, error) {
	// we could some day find the sha from the version stream
	// though using head is a good default really
	return defaultSHA, nil
}

// GetMandatoryTaskSpec returns the mandatory first task spec in the given PipelineRun
func GetMandatoryTaskSpec(pr *v1beta1.PipelineRun) (*v1beta1.TaskSpec, error) {
	ps := pr.Spec.PipelineSpec
	if ps == nil {
		return nil, errors.Errorf("no spec.pipelineSpec")
	}
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec != nil {
			return &pt.TaskSpec.TaskSpec, nil
		}
	}
	return nil, errors.Errorf("no spec.tasks.taskSpec found")
}
