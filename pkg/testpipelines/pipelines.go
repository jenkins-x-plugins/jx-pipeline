package testpipelines

import (
	"context"
	"testing"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	typev1 "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/typed/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// CreateTestPipelineActivity creates a PipelineActivity with the given arguments
func CreateTestPipelineActivity(ctx context.Context, jxClient versioned.Interface, ns, folder, repo, branch, build string) (*v1.PipelineActivity, error) {
	resources := jxClient.JenkinsV1().PipelineActivities(ns)
	key := newPromoteStepActivityKey(folder, repo, branch, build)
	a, _, err := key.GetOrCreate(jxClient, ns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create activity key")
	}
	version := "1.0." + build
	a.Spec.GitOwner = folder
	a.Spec.GitRepository = repo
	a.Spec.GitURL = "https://fake.git/" + folder + "/" + repo + ".git"
	a.Spec.Version = version
	_, err = resources.Update(ctx, a, metav1.UpdateOptions{})
	return a, err
}

// CreateTestPipelineActivityWithTime creates a PipelineActivity with the given timestamp and adds it to the list of activities
func CreateTestPipelineActivityWithTime(ctx context.Context, jxClient versioned.Interface, ns, folder, repo, branch, build string, t metav1.Time) (*v1.PipelineActivity, error) {
	resources := jxClient.JenkinsV1().PipelineActivities(ns)
	key := newPromoteStepActivityKey(folder, repo, branch, build)
	a, _, err := key.GetOrCreate(jxClient, ns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create activity key")
	}
	a.Spec.StartedTimestamp = &t
	_, err = resources.Update(ctx, a, metav1.UpdateOptions{})
	return a, err
}

func AssertHasPullRequestForEnv(t *testing.T, ctx context.Context, activities typev1.PipelineActivityInterface, name, envName string) { //nolint:revive
	activity, err := activities.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		assert.NoError(t, err, "Could not find PipelineActivity %s", name)
		return
	}
	for _, step := range activity.Spec.Steps {
		promote := step.Promote
		if promote != nil {
			if promote.Environment == envName {
				failed := false
				pullRequestStep := promote.PullRequest
				if pullRequestStep == nil {
					assert.Fail(t, "No PullRequest object on PipelineActivity %s for Promote step for Environment %s", name, envName)
					failed = true
				}
				u := ""
				if pullRequestStep != nil {
					u = pullRequestStep.PullRequestURL
				}
				log.Logger().Infof("Found Promote PullRequest %s on PipelineActivity %s for Environment %s", u, name, envName)

				if !assert.True(t, u != "", "No PullRequest URL on PipelineActivity %s for Promote step for Environment %s", name, envName) {
					failed = true
				}
				if failed {
					dumpFailedActivity(activity)
				}
				return
			}
		}
	}
	assert.Fail(t, "Missing Promote", "No Promote found on PipelineActivity %s for Environment %s", name, envName)
	dumpFailedActivity(activity)
}

func dumpFailedActivity(activity *v1.PipelineActivity) {
	data, err := yaml.Marshal(activity)
	if err == nil {
		log.Logger().Warnf("YAML: %s", string(data))
	}
}

func newPromoteStepActivityKey(folder, repo, branch, build string) *activities.PromoteStepActivityKey {
	return &activities.PromoteStepActivityKey{
		PipelineActivityKey: activities.PipelineActivityKey{
			Name:     folder + "-" + repo + "-" + branch + "-" + build,
			Pipeline: folder + "/" + repo + "/" + branch,
			Build:    build,
			GitInfo: &giturl.GitRepository{
				Name:         "my-app",
				Organisation: "myorg",
			},
		},
	}
}

func ClearTimestamps(pa *v1.PipelineActivity) {
	pa.Spec.StartedTimestamp = nil
	pa.Spec.CompletedTimestamp = nil
	for i := range pa.Spec.Steps {
		step := &pa.Spec.Steps[i]
		if step.Stage != nil {
			step.Stage.StartedTimestamp = nil
			step.Stage.CompletedTimestamp = nil

			for j := range step.Stage.Steps {
				s2 := &step.Stage.Steps[j]
				s2.StartedTimestamp = nil
				s2.CompletedTimestamp = nil
			}
		}
		if step.Promote != nil {
			step.Promote.StartedTimestamp = nil
			step.Promote.CompletedTimestamp = nil
		}
		if step.Preview != nil {
			step.Preview.StartedTimestamp = nil
			step.Preview.CompletedTimestamp = nil
		}
	}
}
