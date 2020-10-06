package testpipelines

import (
	"context"
	"testing"

	v1 "github.com/jenkins-x/jx-api/v3/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	typev1 "github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned/typed/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// CreateTestPipelineActivity creates a PipelineActivity with the given arguments
func CreateTestPipelineActivity(jxClient versioned.Interface, ns string, folder string, repo string, branch string, build string, workflow string) (*v1.PipelineActivity, error) {
	ctx := context.Background()
	resources := jxClient.JenkinsV1().PipelineActivities(ns)
	key := newPromoteStepActivityKey(folder, repo, branch, build)
	a, _, err := key.GetOrCreate(jxClient, ns)
	version := "1.0." + build
	a.Spec.GitOwner = folder
	a.Spec.GitRepository = repo
	a.Spec.GitURL = "https://fake.git/" + folder + "/" + repo + ".git"
	a.Spec.Version = version
	a.Spec.Workflow = workflow
	_, err = resources.Update(ctx, a, metav1.UpdateOptions{})
	return a, err
}

// CreateTestPipelineActivityWithTime creates a PipelineActivity with the given timestamp and adds it to the list of activities
func CreateTestPipelineActivityWithTime(jxClient versioned.Interface, ns string, folder string, repo string, branch string, build string, workflow string, t metav1.Time) (*v1.PipelineActivity, error) {
	ctx := context.Background()
	resources := jxClient.JenkinsV1().PipelineActivities(ns)
	key := newPromoteStepActivityKey(folder, repo, branch, build)
	a, _, err := key.GetOrCreate(jxClient, ns)
	a.Spec.StartedTimestamp = &t
	_, err = resources.Update(ctx, a, metav1.UpdateOptions{})
	return a, err
}

func AssertHasPullRequestForEnv(t *testing.T, activities typev1.PipelineActivityInterface, name string, envName string) {
	ctx := context.Background()
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
				u := pullRequestStep.PullRequestURL
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

func newPromoteStepActivityKey(folder string, repo string, branch string, build string) *activities.PromoteStepActivityKey {
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
