package cloud

const (
	GKE        = "gke"
	OKE        = "oke"
	EKS        = "eks"
	AKS        = "aks"
	AWS        = "aws"
	PKS        = "pks"
	IKS        = "iks"
	KUBERNETES = "kubernetes"
	OPENSHIFT  = "openshift"
	ICP        = "icp"
	JX_INFRA   = "jx-infra"
	ALIBABA    = "alibaba"
)

// KubernetesProviders list of all available Kubernetes providers
var KubernetesProviders = []string{GKE, OKE, AKS, AWS, EKS, KUBERNETES, IKS, OPENSHIFT, JX_INFRA, PKS, ICP, ALIBABA}
