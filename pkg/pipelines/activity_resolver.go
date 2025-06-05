package pipelines

import (
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/lighthouse/pkg/clients"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// ActivityResolver a helper object to map PipelineRun objects to PipelineActivity resources
type ActivityResolver struct {
	activities []v1.PipelineActivity
	index      map[string]*v1.PipelineActivity
}

// NewActivityResolver creates a new resolver
func NewActivityResolver(activities []v1.PipelineActivity) *ActivityResolver {
	index := map[string]*v1.PipelineActivity{}
	for i := range activities {
		a := &activities[i]
		index[a.Name] = a
	}
	return &ActivityResolver{
		activities: activities,
		index:      index,
	}
}

// ToPipelineActivity converts the given PipelineRun to a PipelineActivity
func (r *ActivityResolver) ToPipelineActivity(pr *pipelinev1.PipelineRun) *v1.PipelineActivity {
	paName := ToPipelineActivityName(pr, r.activities)
	if paName == "" {
		return nil
	}
	pa := r.index[paName]
	if pa == nil {
		pa = &v1.PipelineActivity{}
		pa.Name = paName
		r.index[paName] = pa
	}
	tektonclient, _, _, _, err := clients.GetAPIClients()
	if err != nil {
		return nil
	}
	ToPipelineActivity(tektonclient, pr, pa, false)
	return pa
}
