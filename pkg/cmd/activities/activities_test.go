// +build unit

package activities_test

import (
	"context"
	"strings"
	"testing"
	"time"

	fakejx "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-pipeline/pkg/cmd/activities"
	"github.com/jenkins-x/jx-pipeline/pkg/testpipelines"
	"github.com/stretchr/testify/require"
	faketekton "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetActivity(t *testing.T) {
	ns := "jx"
	stdout := &strings.Builder{}

	jxClient := fakejx.NewSimpleClientset()

	ctx := context.Background()
	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "1", v1.Date(2019, time.October, 10, 23, 0, 0, 0, time.UTC))
	testpipelines.CreateTestPipelineActivityWithTime(ctx, jxClient, ns, "jx-testing", "jx-testing", "job", "2", v1.Date(2019, time.January, 10, 23, 0, 0, 0, time.UTC))

	kubeClient := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		},
	)

	_, options := activities.NewCmdActivities()
	options.JXClient = jxClient
	options.KubeClient = kubeClient
	options.TektonClient = faketekton.NewSimpleClientset()
	options.Namespace = ns
	options.Out = stdout

	err := options.Run()
	require.NoError(t, err, "failed to run command")

	text := stdout.String()

	t.Logf("got: %s\n", text)

	orderedExpectedStrings := []string{
		"STARTED AGO DURATION STATUS",
		"jx-testing/jx-testing/job #1",
		"jx-testing/jx-testing/job #2",
	}

	for _, expected := range orderedExpectedStrings {
		require.Contains(t, text, expected)

		// strip the text
		idx := strings.Index(text, expected)
		text = text[idx+len(expected):]
	}
}
