// +build unit

package get_test

import (
	"testing"

	"github.com/jenkins-x/jx-pipeline/pkg/cmd/get"
	"github.com/jenkins-x/jx/v2/pkg/tekton"
	"github.com/jenkins-x/jx/v2/pkg/tekton/syntax"
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
				tekton.LabelRepo:    repo,
				tekton.LabelBranch:  branch,
				tekton.LabelOwner:   owner,
				tekton.LabelContext: context,
			},
		},
		Spec: v1beta1.PipelineRunSpec{
			Params: []v1beta1.Param{
				{
					Name:  "version",
					Value: syntax.StringParamValue("v1"),
				},
				{
					Name:  "build_id",
					Value: syntax.StringParamValue("1"),
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

			err := o.Run()

			// Execution should not error out
			assert.NoError(t, err, "execute get pipelines")
		})
	}

}
