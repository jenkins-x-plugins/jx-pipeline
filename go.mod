module github.com/jenkins-x-plugins/jx-pipeline

require (
	github.com/GoogleContainerTools/kpt v0.37.1
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/charmbracelet/bubbletea v0.13.1
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/fatih/color v1.10.0
	github.com/gerow/pager v0.0.0-20190420205801-6d4a2327822f
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/jenkins-x-plugins/jx-gitops v0.4.3
	github.com/jenkins-x/go-scm v1.11.4
	github.com/jenkins-x/jx-api/v4 v4.3.3
	github.com/jenkins-x/jx-helpers/v3 v3.2.3
	github.com/jenkins-x/jx-kube-client/v3 v3.0.2
	github.com/jenkins-x/jx-logging/v3 v3.0.6
	github.com/jenkins-x/lighthouse-client v0.0.411
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/tektoncd/pipeline v0.26.0
	gocloud.dev v0.21.0
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/pkg v0.0.0-20210730172132-bb4aaf09c430
	sigs.k8s.io/yaml v1.2.0
)

replace (
	// helm dependencies
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible

	// lets override the go-scm version from tektoncd
	github.com/jenkins-x/go-scm => github.com/jenkins-x/go-scm v1.11.4

	// for the PipelineRun debug fix see: https://github.com/tektoncd/pipeline/pull/4145
	github.com/tektoncd/pipeline => github.com/jstrachan/pipeline v0.21.1-0.20210811150720-45a86a5488af

	k8s.io/api => k8s.io/api v0.20.7
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.7
	k8s.io/client-go => k8s.io/client-go v0.20.7

	knative.dev/pkg => knative.dev/pkg v0.0.0-20210730172132-bb4aaf09c430
)

go 1.15
