package env_test

import (
	"context"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cmd/env"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	faketekton "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
)

func TestPodEnvVars(t *testing.T) {
	ns := "jx"
	_, o := env.NewCmdPipelineEnv()

	o.KubeClient = fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "single",
				Namespace: ns,
			},
			Data: map[string]string{
				"mySingleCmValue": "singleCmValue",
				"random":          "value",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "all",
				Namespace: ns,
			},
			Data: map[string]string{
				"CM_V1": "cmValue1",
				"CM_V2": "cmValue2",
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "single",
				Namespace: ns,
			},
			Data: map[string][]byte{
				"mySingleSecretValue": []byte("singleSecretValue"),
				"random":              []byte("value"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "all",
				Namespace: ns,
			},
			Data: map[string][]byte{
				"SEC_V1": []byte("secValue1"),
				"SEC_V2": []byte("secValue2"),
			},
			StringData: map[string]string{
				"SEC_SD1": "secSDValue1",
				"SEC_SD2": "secSDValue2",
			},
		})
	o.TektonClient = faketekton.NewSimpleClientset()
	o.Namespace = ns
	o.BatchMode = true
	o.Ctx = context.Background()

	containerName := "cheese"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mypod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: containerName,
					Env: []corev1.EnvVar{
						{
							Name:  "SIMPLE_VAR",
							Value: "simpleVarValue",
						},
						{
							Name: "FROM_CONFIG_KEY",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "single",
									},
									Key: "mySingleCmValue",
								},
							},
						},
						{
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "all",
									},
								},
							},
						},

						{
							Name: "FROM_SECRET",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "single",
									},
									Key: "mySingleSecretValue",
								},
							},
						},
						{
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "all",
									},
								},
							},
						},
					},
					EnvFrom: []corev1.EnvFromSource{},
				},
			},
		},
	}
	ev, err := o.PodEnvVars(pod, containerName)
	assert.NoError(t, err, "failed to invoke PodEnvVars()")
	require.NotNil(t, ev, "no environment variables returned")

	// Env
	assertEnvValue(t, ev, "SIMPLE_VAR", "simpleVarValue")

	// Env ConfigMap
	assertEnvValue(t, ev, "FROM_CONFIG_KEY", "singleCmValue")
	assertEnvValue(t, ev, "CM_V1", "cmValue1")
	assertEnvValue(t, ev, "CM_V2", "cmValue2")

	// Env Secret
	assertEnvValue(t, ev, "FROM_SECRET", "singleSecretValue")
	assertEnvValue(t, ev, "SEC_V1", "secValue1")
	assertEnvValue(t, ev, "SEC_V2", "secValue2")
	assertEnvValue(t, ev, "SEC_SD1", "secSDValue1")
	assertEnvValue(t, ev, "SEC_SD2", "secSDValue2")

}

func assertEnvValue(t *testing.T, ev map[string]string, name string, expected string) {
	actual := ev[name]
	assert.Equal(t, expected, actual, "for environment variable %s", name)
	t.Logf("%s=%s\n", name, actual)
}
