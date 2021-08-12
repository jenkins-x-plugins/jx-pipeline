// +build unit

package breakpoint_test

import (
	"context"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/breakpoint"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/testpipelines"
	fakejx "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestPipelineBreakpoint(t *testing.T) {
	ns := "jx"

	_, o := breakpoint.NewCmdPipelineBreakpoint()
	ctx := o.GetContext()

	jxClient := fakejx.NewSimpleClientset()
	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "1", metav1.Date(2019, time.October, 10, 23, 0, 0, 0, time.UTC))
	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "2", metav1.Date(2019, time.January, 10, 23, 0, 0, 0, time.UTC))

	kubeClient := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		},
	)

	o.KubeClient = kubeClient
	o.JXClient = jxClient
	o.Namespace = ns
	o.BatchMode = true
	o.Ctx = context.Background()

	err := o.Run()
	require.NoError(t, err, "failed to run")
}
