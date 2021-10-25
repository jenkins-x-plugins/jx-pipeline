package wait_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/wait"
	jxV1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	fakejx "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/fake"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var testCases = []struct {
	description string
	owner       string
	repo        string
	namespace   string
	cmFile      string
	srFile      string
}{
	{
		description: "Test repo with no underscore",
		owner:       "jenkins-x-plugins",
		repo:        "jx-pipeline",
		namespace:   "jx",
		cmFile:      "cm.yaml",
		srFile:      "sr.yaml",
	},
	{
		description: "Test repo with underscore",
		owner:       "jenkins-x-plugins",
		repo:        "jx-pipeline", // wait function always receives repo names with _ replaced by -
		namespace:   "jx",
		cmFile:      "cm_underscore.yaml",
		srFile:      "sr_underscore.yaml",
	},
}

func TestPipelineWait(t *testing.T) {
	for _, v := range testCases {
		t.Log(v.description)
		cmd, o := wait.NewCmdPipelineWait()

		// Set owner, repo and namespace
		o.Owner = v.owner
		o.Repository = v.repo
		o.Namespace = v.namespace

		// Load configmap from test files in testData folder
		cmFile := filepath.Join("testdata", v.cmFile)
		require.FileExists(t, cmFile)
		cm := &v1.ConfigMap{}
		err := yamls.LoadFile(cmFile, cm)
		assert.NoError(t, err)

		// Load sourcerepo from test files in testData folder
		srFile := filepath.Join("testdata", v.srFile)
		require.FileExists(t, srFile)
		sr := &jxV1.SourceRepository{}
		err = yamls.LoadFile(srFile, sr)
		assert.NoError(t, err)

		// Fake kubeclient
		kubeClient := fake.NewSimpleClientset(cm)
		o.KubeClient = kubeClient

		// Fake JX client
		jxClient := fakejx.NewSimpleClientset(sr)
		o.JXClient = jxClient

		// Set wait and poll duration, as we dont want to wait for long in unit tests
		// ToDo: May be find a better way to handle this?
		o.WaitDuration = 100 * time.Millisecond
		o.PollPeriod = 50 * time.Millisecond
		o.Ctx = context.TODO()
		err = cmd.Execute()
		assert.NoError(t, err)
	}
}
