package lighthouses

import (
	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	lhclient "github.com/jenkins-x/lighthouse/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
)

// LazyCreateLHClient lazy creates the jx client if its not defined
func LazyCreateLHClient(client lhclient.Interface) (lhclient.Interface, error) {
	if client != nil {
		return client, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, errors.Wrap(err, "failed to get kubernetes config")
	}
	client, err = lhclient.NewForConfig(cfg)
	if err != nil {
		return client, errors.Wrap(err, "error building lighthouse clientset")
	}
	return client, nil
}
