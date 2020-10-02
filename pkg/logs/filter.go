package logs

import (
	"net/url"
	"strings"

	v1 "github.com/jenkins-x/jx-api/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/v2/pkg/gits"
	"github.com/jenkins-x/jx/v2/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// BuildPodInfoFilter for filtering pipelines / PipelineRuns
type BuildPodInfoFilter struct {
	Owner      string
	Repository string
	Branch     string
	Build      string
	Filter     string
	Pod        string
	Pending    bool
	Context    string
	GitURL     string
}

// Matches returns true if the PipelineActivity matches the filter
func (o *BuildPodInfoFilter) Matches(pa *v1.PipelineActivity) bool {
	if o == nil {
		return true
	}
	ps := &pa.Spec

	if o.Owner != "" && o.Owner != ps.GitOwner {
		return false
	}
	if o.Repository != "" && o.Repository != ps.GitRepository {
		return false
	}
	if o.Repository != "" && o.Repository != ps.GitRepository {
		return false
	}
	if o.Branch != "" && o.Branch != ps.GitBranch {
		return false
	}
	if o.Build != "" && o.Build != ps.Build {
		return false
	}
	if o.Context != "" && o.Context != ps.Context {
		return false
	}
	if o.Pending && ps.Status.IsTerminated() {
		return false
	}
	return true
}

// AddFlags adds the CLI flags for filtering
func (o *BuildPodInfoFilter) AddFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&o.Pending, "pending", "p", false, "Only display logs which are currently pending to choose from if no build name is supplied")
	cmd.Flags().StringVarP(&o.Filter, "filter", "f", "", "Filters all the available jobs by those that contain the given text")
	cmd.Flags().StringVarP(&o.Owner, "owner", "o", "", "Filters the owner (person/organisation) of the repository")
	cmd.Flags().StringVarP(&o.Repository, "repo", "r", "", "Filters the build repository")
	cmd.Flags().StringVarP(&o.Branch, "branch", "", "", "Filters the branch")
	cmd.Flags().StringVarP(&o.Build, "build", "", "", "The build number to view")
	cmd.Flags().StringVarP(&o.Pod, "pod", "", "", "The pod name to view")
	cmd.Flags().StringVarP(&o.GitURL, "giturl", "g", "", "The git URL to filter on. If you specify a link to a github repository or PR we can filter the query of build pods accordingly")
	cmd.Flags().StringVarP(&o.Context, "context", "", "", "Filters the context of the build")
}

// Validate validates the settings
func (o *BuildPodInfoFilter) Validate() error {
	u := o.GitURL
	if u != "" && (o.Owner == "" || o.Repository == "" || o.Branch == "") {
		branch := ""
		u2, err := url.Parse(u)
		if err == nil {
			paths := strings.Split(u2.Path, "/")
			l := len(paths)
			if l > 3 {
				if paths[l-2] == "pull" || paths[l-2] == "pulls" {
					branch = "PR-" + paths[l-1]

					// lets remove the pulls path
					if paths[0] == "" {
						paths[0] = "/"
					}
					u2.Path = util.UrlJoin(paths[0:3]...)
					u = u2.String()
				}
			}
		}
		gitInfo, err := gits.ParseGitURL(u)
		if err != nil {
			return errors.Wrapf(err, "could not parse GitURL: %s", u)
		}
		o.Owner = gitInfo.Organisation
		o.Repository = gitInfo.Name
		if branch != "" {
			o.Branch = branch
		}
	}
	return nil
}
