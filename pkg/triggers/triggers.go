package triggers

import (
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/config/job"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// LoadLighthouseConfig loads the lighthouse configuration from the given ConfigMap namespace and name
func LoadLighthouseConfig(kubeClient kubernetes.Interface, ns string, name string, allowEmpty bool) (*config.Config, error) {
	cm, err := kubeClient.CoreV1().ConfigMaps(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if allowEmpty {
				return CreateEmptyConfig(), nil
			}
			return nil, errors.Errorf("no ConfigMap %s exists in namespace %s. you can switch namespaces via: jx ns", name, ns)
		}
		return nil, errors.Wrapf(err, "failed to find ConfigMap %s in namespace %s", name, ns)
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
		return nil, errors.Errorf("lighthouse ConfigMap %s in namespace %s does not contain key %s", name, ns, key)
	}

	cfg, err := LoadLighthouseConfigYAML(configYaml)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load lighthouse config")
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
		return nil, errors.Wrapf(err, "failed to parse Lighthouse config YAML")
	}
	return cfg, nil
}
