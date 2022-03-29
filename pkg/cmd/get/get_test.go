//go:build unit
// +build unit

package get_test

import (
	"context"
	"testing"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/get"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/tektonlog"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	faketekton "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testDevNameSpace = "jx-test"

func pipelineRun(ns, repo, branch, owner, context string, now metav1.Time) *tektonv1beta1.PipelineRun {
	return &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "PR1",
			Namespace: ns,
			Labels: map[string]string{
				tektonlog.LabelRepo:    repo,
				tektonlog.LabelBranch:  branch,
				tektonlog.LabelOwner:   owner,
				tektonlog.LabelContext: context,
			},
		},
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name: "version",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "v1",
					},
				},
				{
					Name: "build_id",
					Value: v1beta1.ArrayOrString{
						Type:      v1beta1.ParamTypeString,
						StringVal: "1",
					},
				},
			},
		},
		Status: v1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				CompletionTime: &now,
			},
		},
	}
}

var pipelineCases = []struct {
	desc      string
	namespace string
	repo      string
	branch    string
	owner     string
	context   string
}{
	{"", testDevNameSpace, "testRepo", "testBranch", "testOwner", "testContext"},
	{"", testDevNameSpace, "testRepo", "testBranch", "testOwner", ""},
}

func TestExecuteGetPipelines(t *testing.T) {
	for _, v := range pipelineCases {
		t.Run(v.desc, func(t *testing.T) {

			_, o := get.NewCmdPipelineGet()

			o.KubeClient = fake.NewSimpleClientset()
			o.TektonClient = faketekton.NewSimpleClientset(pipelineRun(v.namespace, v.repo, v.branch, v.owner, v.context, metav1.Now()))
			o.Namespace = v.namespace
			o.BatchMode = true
			o.Ctx = context.Background()

			err := o.Run()

			// Execution should not error out
			assert.NoError(t, err, "execute get pipelines")
		})
	}

}
