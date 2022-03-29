//go:build unit
// +build unit

package breakpoint_test

import (
	"context"
	"testing"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/breakpoint"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/testpipelines"
	fakejx "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	fakelh "github.com/jenkins-x/lighthouse-client/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"k8s.io/client-go/kubernetes/fake"
)

func TestPipelineBreakpoint(t *testing.T) {
	ns := "jx"

	_, o := breakpoint.NewCmdPipelineBreakpoint()
	ctx := o.GetContext()

	jxClient := fakejx.NewSimpleClientset()
	lhClient := fakelh.NewSimpleClientset()
	kubeClient := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		},
	)

	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "1", metav1.Date(2019, time.October, 10, 23, 0, 0, 0, time.UTC))
	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "2", metav1.Date(2019, time.January, 10, 23, 0, 0, 0, time.UTC))

	setupAndRun := func() {
		o.KubeClient = kubeClient
		o.JXClient = jxClient
		o.LHClient = lhClient
		o.Namespace = ns
		o.BatchMode = true
		o.Ctx = context.Background()

		err := o.Run()
		require.NoError(t, err, "failed to run")
	}

	setupAndRun()

	bpList, err := o.LHClient.LighthouseV1alpha1().LighthouseBreakpoints(ns).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "should not have failed to list LighthouseBreakpoint resources")
	require.Len(t, bpList.Items, 1, "should have created 1 LighthouseBreakpoint")
	bp := bpList.Items[0]
	data, err := yaml.Marshal(bp)
	require.NoError(t, err, "failed to marshal Breakpoint to YAML %#v", bp)

	t.Logf("created Breakpoint YAML:\n%s\n", string(data))

	// lets run again and make sure we delete the breakpoint
	_, o = breakpoint.NewCmdPipelineBreakpoint()
	setupAndRun()

	bpList, err = o.LHClient.LighthouseV1alpha1().LighthouseBreakpoints(ns).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "should not have failed to list LighthouseBreakpoint resources")
	require.Emptyf(t, bpList.Items, "should have deleted the LighthouseBreakpoint")
}
