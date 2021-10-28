package sourcerepos

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	jenkinsio "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetRepositoryGitURL returns the git repository clone URL
func GetRepositoryGitURL(s *v1.SourceRepository) (string, error) {
	spec := s.Spec
	provider := spec.Provider
	owner := spec.Org
	repo := spec.Repo
	if spec.HTTPCloneURL == "" {
		if spec.ProviderKind == "bitbucketserver" {
			provider = stringhelpers.UrlJoin(provider, "scm")
		}
		if provider == "" {
			return spec.HTTPCloneURL, fmt.Errorf("missing provider in SourceRepository %s", s.Name)
		}
		if owner == "" {
			return spec.HTTPCloneURL, fmt.Errorf("missing org in SourceRepository %s", s.Name)
		}
		if repo == "" {
			return spec.HTTPCloneURL, fmt.Errorf("missing repo in SourceRepository %s", s.Name)
		}
		spec.HTTPCloneURL = stringhelpers.UrlJoin(provider, owner, repo) + ".git"
	}
	return spec.HTTPCloneURL, nil
}

// FindSourceRepositoryWithoutProvider returns a SourceRepository for the given namespace, owner and repo name.
// If no SourceRepository is found, return nil.
func FindSourceRepositoryWithoutProvider(ctx context.Context, jxClient versioned.Interface, ns, owner, name string) (*v1.SourceRepository, error) {
	return FindSourceRepository(ctx, jxClient, ns, owner, name, "")
}

// FindSourceRepository returns a SourceRepository for the given namespace, owner, repo name, and (optional) provider name.
// If no SourceRepository is found, return nil.
func FindSourceRepository(ctx context.Context, jxClient versioned.Interface, ns, owner, name, providerName string) (*v1.SourceRepository, error) {
	// Look up by resource name is retained for compatibility with SourceRepositorys created before they were always created with labels
	resourceName := naming.ToValidName(owner + "-" + name)
	repo, err := jxClient.JenkinsV1().SourceRepositories(ns).Get(ctx, resourceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Logger().Debugf("could not find SourceRepository %s in namespace %s", name, ns)

			repos, reposErr := jxClient.JenkinsV1().SourceRepositories(ns).List(ctx, metav1.ListOptions{})
			if reposErr != nil {
				return nil, errors.Wrapf(reposErr, "listing SourceRepository resources in namespace %s", ns)
			}

			for i := range repos.Items {
				r := &repos.Items[i]
				if r.Spec.Org == owner && r.Spec.Repo == name {
					return r, nil
				}
			}
			return nil, nil
		}
		return nil, errors.Wrapf(err, "getting SourceRepository %s in namespace %s", resourceName, ns)
	}
	return repo, nil
}

// GetOrCreateSourceRepositoryCallback gets or creates the SourceRepository for the given repository name and
// organisation invoking the given callback to modify the resource before create/udpate
func GetOrCreateSourceRepositoryCallback(ctx context.Context, jxClient versioned.Interface, ns, name, organisation, providerURL string, callback func(*v1.SourceRepository)) (*v1.SourceRepository, error) {
	resourceName := naming.ToValidName(organisation + "-" + name)

	repositories := jxClient.JenkinsV1().SourceRepositories(ns)

	providerName := ToProviderName(providerURL)

	foundSr, err := FindSourceRepository(ctx, jxClient, ns, organisation, name, providerName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find existing SourceRepository")
	}

	// If we did not find an existing SourceRepository for this org/repo, create one
	if foundSr == nil {
		return createSourceRepositoryCallback(ctx, jxClient, ns, name, organisation, providerURL, callback)
	}

	// If we did find a SourceRepository, use that as our basis and see if we need to update it.
	description := fmt.Sprintf("Imported application for %s/%s", organisation, name)

	srCopy := foundSr.DeepCopy()
	srCopy.Name = foundSr.Name
	srCopy.Spec.Description = description
	srCopy.Spec.Org = organisation
	srCopy.Spec.Provider = providerURL
	srCopy.Spec.Repo = name

	srCopy.Labels = map[string]string{}
	for k, v := range foundSr.Labels {
		srCopy.Labels[k] = v
	}
	srCopy.Labels[v1.LabelProvider] = providerName
	srCopy.Labels[v1.LabelOwner] = organisation
	srCopy.Labels[v1.LabelRepository] = name

	if callback != nil {
		callback(srCopy)
	}
	srCopy.Sanitize()

	// If we don't need to update the found SourceRepository, return it.
	if reflect.DeepEqual(srCopy.Spec, foundSr.Spec) && reflect.DeepEqual(srCopy.Labels, foundSr.Labels) {
		return foundSr, nil
	}

	// Otherwise, update the SourceRepository and return it.
	answer, err := repositories.Update(ctx, srCopy, metav1.UpdateOptions{})
	if err != nil {
		return answer, errors.Wrapf(err, "failed to update SourceRepository %s", resourceName)
	}
	answer, err = repositories.Get(ctx, foundSr.Name, metav1.GetOptions{})
	if err != nil {
		return answer, errors.Wrapf(err, "failed to get SourceRepository %s", resourceName)
	}

	return answer, nil
}

// GetOrCreateSourceRepository gets or creates the SourceRepository for the given repository name and organisation
func GetOrCreateSourceRepository(ctx context.Context, jxClient versioned.Interface, ns, name, organisation, providerURL string) (*v1.SourceRepository, error) {
	return GetOrCreateSourceRepositoryCallback(ctx, jxClient, ns, name, organisation, providerURL, nil)
}

// ToProviderName takes the git URL and converts it to a provider name which can be used as a label selector
func ToProviderName(gitURL string) string {
	if gitURL == "" {
		return ""
	}
	u, err := url.Parse(gitURL)
	if err == nil {
		host := strings.TrimSuffix(u.Host, ".com")
		return naming.ToValidName(host)
	}
	idx := strings.Index(gitURL, "://")
	if idx > 0 {
		gitURL = gitURL[idx+3:]
	}
	gitURL = strings.TrimSuffix(gitURL, "/")
	gitURL = strings.TrimSuffix(gitURL, ".com")
	return naming.ToValidName(gitURL)
}

// createSourceRepositoryCallback creates a repo, returning the created repo and an error if it couldn't be created
func createSourceRepositoryCallback(ctx context.Context, client versioned.Interface, namespace, name, organisation, providerURL string, callback func(*v1.SourceRepository)) (*v1.SourceRepository, error) {
	resourceName := naming.ToValidName(organisation + "-" + name)

	description := fmt.Sprintf("Imported application for %s/%s", organisation, name)

	providerName := ToProviderName(providerURL)
	labels := map[string]string{
		v1.LabelProvider:   providerName,
		v1.LabelOwner:      organisation,
		v1.LabelRepository: name,
	}

	sr := &v1.SourceRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SourceRepository",
			APIVersion: jenkinsio.GroupName + "/" + jenkinsio.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   resourceName,
			Labels: labels,
		},
		Spec: v1.SourceRepositorySpec{
			Description:  description,
			Org:          organisation,
			Provider:     providerURL,
			ProviderName: providerName,
			Repo:         name,
		},
	}
	if callback != nil {
		callback(sr)
	}
	sr.Sanitize()
	answer, err := client.JenkinsV1().SourceRepositories(namespace).Create(ctx, sr, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new SourceRepository for organisation %s and repository %s", organisation, name)
	}

	return answer, nil
}

// IsRemoteEnvironmentRepository returns true if the given repository is a remote environment
func IsRemoteEnvironmentRepository(environments map[string]*v1.Environment, repository *v1.SourceRepository) bool {
	gitURL, err := GetRepositoryGitURL(repository)
	if err != nil {
		return false
	}
	u2 := gitURL + ".git"

	for _, env := range environments {
		if env.Spec.Kind != v1.EnvironmentKindTypePermanent {
			continue
		}
		if env.Spec.Source.URL == gitURL || env.Spec.Source.URL == u2 {
			if env.Spec.RemoteCluster {
				return true
			}
		}
	}
	return false
}

// IsIncludedInTheGivenEnvs returns true if the given repository is an environment repository
func IsIncludedInTheGivenEnvs(environments map[string]*v1.Environment, repository *v1.SourceRepository) bool {
	gitURL, err := GetRepositoryGitURL(repository)
	if err != nil {
		return false
	}
	u2 := gitURL + ".git"

	for _, env := range environments {
		if env.Spec.Source.URL == gitURL || env.Spec.Source.URL == u2 {
			return true
		}
	}
	return false
}
