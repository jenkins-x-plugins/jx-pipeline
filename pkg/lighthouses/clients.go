package lighthouses

import (
	"fmt"

	"github.com/jenkins-x/jx-kube-client/v3/pkg/kubeclient"
	lhclient "github.com/jenkins-x/lighthouse-client/pkg/client/clientset/versioned"
)

// LazyCreateLHClient lazy creates the jx client if its not defined
func LazyCreateLHClient(client lhclient.Interface) (lhclient.Interface, error) {
	if client != nil {
		return client, nil
	}
	f := kubeclient.NewFactory()
	cfg, err := f.CreateKubeConfig()
	if err != nil {
		return client, fmt.Errorf("failed to get kubernetes config: %w", err)
	}
	client, err = lhclient.NewForConfig(cfg)
	if err != nil {
		return client, fmt.Errorf("error building lighthouse clientset: %w", err)
	}
	return client, nil
}
