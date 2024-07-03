package triggers

import (
	"context"
	"fmt"

	"github.com/jenkins-x/lighthouse-client/pkg/config"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// LoadLighthouseConfig loads the lighthouse configuration from the given ConfigMap namespace and name
func LoadLighthouseConfig(ctx context.Context, kubeClient kubernetes.Interface, ns, name string, allowEmpty bool) (*config.Config, error) {
	cm, err := kubeClient.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if allowEmpty {
				return CreateEmptyConfig(), nil
			}
			return nil, fmt.Errorf("no ConfigMap %s exists in namespace %s. you can switch namespaces via: jx ns", name, ns)
		}
		return nil, fmt.Errorf("failed to find ConfigMap %s in namespace %s: %w", name, ns, err)
	}
	key := "config.yaml"
	configYaml := ""
	if cm.Data != nil {
		configYaml = cm.Data[key]
	}
	if configYaml == "" {
		if allowEmpty {
			return CreateEmptyConfig(), nil
		}
		return nil, fmt.Errorf("lighthouse ConfigMap %s in namespace %s does not contain key %s", name, ns, key)
	}
	cfg, err := LoadLighthouseConfigYAML(configYaml)
	if err != nil {
		return nil, fmt.Errorf("failed to load lighthouse config: %w", err)
	}
	return cfg, nil
}

// CreateEmptyConfig creates a default empty configuration
func CreateEmptyConfig() *config.Config {
	return &config.Config{
		JobConfig: config.JobConfig{
			Presubmits:  map[string][]job.Presubmit{},
			Postsubmits: map[string][]job.Postsubmit{},
		},
	}
}

// LoadLighthouseConfig loads the lighthouse configuration
func LoadLighthouseConfigYAML(configYaml string) (*config.Config, error) {
	// lets avoid lighthouse really changing the log level
	lvl := logrus.GetLevel()
	logrus.SetLevel(logrus.ErrorLevel)
	defer logrus.SetLevel(lvl)

	cfg, err := config.LoadYAMLConfig([]byte(configYaml))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Lighthouse config YAML: %w", err)
	}
	return cfg, nil
}
