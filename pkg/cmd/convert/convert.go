package convert

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x/jx-pipeline/pkg/pipelines/processor"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kpt/pkg/kptfile"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions
	ScmOptions scmhelpers.Options

	Namespace   string
	TasksFolder string
	Format      string
	CatalogSHA  string
	Catalog     bool
	Resolver    *inrepo.UsesResolver
	Processor   *processor.UsesMigrator

	// KptPath is the imported path in the catalog for repository pipelines
	KptPath string
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Converts the pipelines to use the 'image: uses:sourceURI' include mechanism

	So that pipelines are smaller, simpler and easier to upgrade pipelines with the version stream
`)

	cmdExample = templates.Examples(`
		# Convert a repository created using the alpha/beta of v3 
        # to use the nice new uses: syntax 
		jx pipeline convert

		# Convert a pipeline catalog to the uses syntax and layout
		jx pipeline convert --catalog
	`)
)

// NewCmdPipelineConvert creates the command
func NewCmdPipelineConvert() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "convert",
		Short:   "Converts the pipelines to use the 'image: uses:sourceURI' include mechanism",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"kill"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.ScmOptions.DiscoverFromGit = true
	cmd.Flags().StringVarP(&o.ScmOptions.Dir, "dir", "d", ".", "The directory to look for the .lighthouse folder")
	cmd.Flags().StringVarP(&o.TasksFolder, "tasks-dir", "", "tasks", "The directory name to store the original tasks before we convert to uses: notation")
	cmd.Flags().StringVarP(&o.CatalogSHA, "sha", "s", "HEAD", "The default catalog SHA to use when resolving catalog pipelines to reuse")
	cmd.Flags().BoolVarP(&o.Catalog, "catalog", "c", false, "If converting a catalog we look in the packs folder to recursively find all '.lighthouse' folders")

	return cmd, o
}

// Validate verifies settings
func (o *Options) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate base options")
	}
	if o.Resolver == nil {
		if o.Catalog {
			o.Resolver, err = lighthouses.CreateResolver(&o.ScmOptions)
			if err != nil {
				return errors.Wrapf(err, "failed to create a UsesResolver")
			}
		} else {
			// lets discover the resolver for each lighthouse folder using the Kptfile
		}
	}
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	err := o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	rootDir := o.ScmOptions.Dir
	if o.Catalog {
		if o.Processor == nil {
			o.Processor = processor.NewUsesMigrator(rootDir, o.TasksFolder, o.ScmOptions.Owner, o.ScmOptions.Repository, o.Catalog)
		}
		packsDir := filepath.Join(rootDir, "packs")
		err := filepath.Walk(packsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info == nil || !info.IsDir() || info.Name() != ".lighthouse" {
				return nil
			}
			return o.ProcessDir(path)
		})
		if err != nil {
			return err
		}
		return nil
	}

	if o.Processor == nil {
		o.Processor = processor.NewUsesMigrator(rootDir, o.TasksFolder, o.ScmOptions.Owner, o.ScmOptions.Repository, o.Catalog)
	}
	dir := filepath.Join(rootDir, ".lighthouse")
	err = o.ProcessDir(dir)
	if err != nil {
		return err
	}
	return nil
}

func (o *Options) ProcessDir(dir string) error {
	fs, err := ioutil.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "failed to read dir %s", dir)
	}
	for _, f := range fs {
		name := f.Name()
		if !f.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}

		triggerDir := filepath.Join(dir, name)
		triggersFile := filepath.Join(triggerDir, "triggers.yaml")
		exists, err := files.FileExists(triggersFile)
		if err != nil {
			return errors.Wrapf(err, "failed to check if file exists %s", triggersFile)
		}
		if !exists {
			continue
		}

		triggers := &triggerconfig.Config{}
		err = yamls.LoadFile(triggersFile, triggers)
		if err != nil {
			return errors.Wrapf(err, "failed to load lighthouse triggers: %s", triggersFile)
		}

		if !o.Catalog {
			o.Resolver, err = o.createNonCatalogResolver(triggerDir)
			if err != nil {
				return errors.Wrapf(err, "failed to create resolver for non catalog in dir %s", triggerDir)
			}
			if o.Resolver == nil {
				log.Logger().Infof("no Kptfile found in dir %s so cannot convert", info(triggerDir))
				continue
			}
		}
		err = o.processTriggerFile(triggers, triggerDir)
		if err != nil {
			return errors.Wrapf(err, "failed to convert pipelines")
		}
	}
	return nil
}

func (o *Options) processTriggerFile(repoConfig *triggerconfig.Config, dir string) error {
	modified := false
	for i := range repoConfig.Spec.Presubmits {
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			err := o.updateCatalogTask(r.SourcePath)
			if err != nil {
				return errors.Wrapf(err, "failed to find catalog pipeline for %s", r.SourcePath)
			}
			path := filepath.Join(dir, r.SourcePath)
			flag, err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				return errors.Wrapf(err, "failed to convert %s", r.SourcePath)
			}
			if flag {
				modified = true
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits {
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			err := o.updateCatalogTask(r.SourcePath)
			if err != nil {
				return errors.Wrapf(err, "failed to find catalog pipeline for %s", r.SourcePath)
			}
			path := filepath.Join(dir, r.SourcePath)
			flag, err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				return errors.Wrapf(err, "failed to convert %s", r.SourcePath)
			}
			if flag {
				modified = true
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	if !o.Catalog && modified {
		// lets remove the kptfile if it exists
		path := filepath.Join(dir, "Kptfile")
		exists, err := files.FileExists(path)
		if err != nil {
			return errors.Wrapf(err, "failed to check if file exists %s", path)
		}
		if exists {
			err = os.RemoveAll(path)
			if err != nil {
				return errors.Wrapf(err, "failed to remove file %s", path)
			}
		}
	}
	return nil
}

func (o *Options) createNonCatalogResolver(triggerDir string) (*inrepo.UsesResolver, error) {
	path := filepath.Join(triggerDir, "Kptfile")
	exists, err := files.FileExists(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check for file %s", path)
	}
	if !exists {
		return nil, nil
	}

	// lets load the
	kf := &kptfile.KptFile{}
	err = yamls.LoadFile(path, kf)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load the kptfile %s", path)
	}

	repoOptions := o.ScmOptions

	// replace owner / repo / tag etc
	git := kf.Upstream.Git
	repoURL := git.Repo
	if repoURL == "" {
		return nil, errors.Errorf("missing upstream.git.repo in %s", path)
	}
	gitInfo, err := giturl.ParseGitURL(repoURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse git URL %s from %s", repoURL, path)
	}
	repoOptions.Owner = gitInfo.Organisation
	repoOptions.Repository = gitInfo.Name

	resolver, err := lighthouses.CreateResolver(&repoOptions)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create the resolver")
	}
	resolver.SHA = "versionStream"
	resolver.Dir = git.Directory
	return resolver, nil
}

// updateCatalogTask lets find the catalog task for the given file so that we can use it
func (o *Options) updateCatalogTask(sourceFile string) error {
	if o.Catalog {
		return nil
	}
	resolver := o.Resolver

	kptPath := filepath.Join(resolver.Dir, sourceFile)
	owner := resolver.OwnerName
	repo := resolver.RepoName
	sha, err := o.getCatalogSHA(owner, repo)
	if err != nil {
		return errors.Wrapf(err, "failed to find SHA for catalog repository %s/%s", owner, repo)
	}

	gu := &inrepo.GitURI{
		Owner:      owner,
		Repository: repo,
		Path:       kptPath,
		SHA:        sha,
	}
	gitURI := gu.String()
	data, err := resolver.GetData(gitURI, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load %s", gitURI)
	}

	pr, err := inrepo.LoadTektonResourceAsPipelineRun(resolver, data)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal catalog YAML file %s", kptPath)
	}
	o.Processor.CatalogTaskSpec, err = findCatalogTaskSpec(pr)
	if err != nil {
		return errors.Wrapf(err, "failed to find catalog task at %s", gitURI)
	}
	return nil
}

func (o *Options) getCatalogSHA(owner string, repo string) (string, error) {
	// we could some day find the sha from the version stream
	// though using head is a good default really
	return o.CatalogSHA, nil
}

func findCatalogTaskSpec(pr *v1beta1.PipelineRun) (*v1beta1.TaskSpec, error) {
	ps := pr.Spec.PipelineSpec
	if ps == nil {
		return nil, errors.Errorf("no spec.pipelineSpec")
	}
	for i := range ps.Tasks {
		pt := &ps.Tasks[i]
		if pt.TaskSpec != nil {
			return &pt.TaskSpec.TaskSpec, nil
		}
	}
	return nil, errors.Errorf("no spec.tasks.taskSpec found")
}
