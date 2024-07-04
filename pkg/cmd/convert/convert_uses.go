package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kpt/pkg/kptfile"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/lighthouses"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines/processor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig/inrepo"

	"github.com/spf13/cobra"
)

// UsesOptions contains the command line options
type UsesOptions struct {
	options.BaseOptions
	lighthouses.ResolverOptions

	Dir           string
	Namespace     string
	TasksFolder   string
	Format        string
	UseSHA        string
	CatalogSHA    string
	Catalog       bool
	UseKptRef     bool
	TriggerCount  int
	Resolver      *inrepo.UsesResolver
	Processor     *processor.UsesMigrator
	CommandRunner cmdrunner.CommandRunner
	GitClient     gitclient.Interface

	// KptPath is the imported path in the catalog for repository pipelines
	KptPath string
}

var (
	info = termcolor.ColorInfo

	usesCmdLong = templates.LongDesc(`
		Converts the pipelines to use the 'image: uses:sourceURI' include mechanism

	So that pipelines are smaller, simpler and easier to upgrade pipelines with the version stream
`)

	usesCmdExample = templates.Examples(`
		# Convert a repository created using the alpha/beta of v3 
        # to use the nice new uses: syntax 
		jx pipeline convert

		# Convert a pipeline catalog to the uses syntax and layout
		jx pipeline convert --catalog
	`)
)

// NewCmdPipelineConvertUses creates the command
func NewCmdPipelineConvertUses() (*cobra.Command, *UsesOptions) {
	o := &UsesOptions{}

	cmd := &cobra.Command{
		Use:     "uses",
		Short:   "Converts the pipelines to use the 'image: uses:sourceURI' include mechanism",
		Long:    usesCmdLong,
		Example: usesCmdExample,
		Run: func(_ *cobra.Command, _ []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.BaseOptions.AddBaseFlags(cmd)
	o.ResolverOptions.AddFlags(cmd)

	cmd.Flags().StringVarP(&o.TasksFolder, "tasks-dir", "", "tasks", "The directory name to store the original tasks before we convert to uses: notation")
	cmd.Flags().StringVarP(&o.CatalogSHA, "sha", "s", "", "The default catalog SHA to use when resolving catalog pipelines to reuse")
	cmd.Flags().StringVarP(&o.UseSHA, "use-sha", "", "", "The catalog SHA to use in the converted pipelines. If not specified defaults to @versionStream")
	cmd.Flags().BoolVarP(&o.Catalog, "catalog", "c", false, "If converting a catalog we look in the packs folder to recursively find all '.lighthouse' folders")
	cmd.Flags().BoolVarP(&o.UseKptRef, "use-kpt-ref", "", true, "Keep the kpt ref value in the uses git URI")

	return cmd, o
}

// Validate verifies settings
func (o *UsesOptions) Validate() error {
	err := o.BaseOptions.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate base options: %w", err)
	}
	if o.Resolver == nil {
		if o.Catalog {
			o.Resolver, err = o.ResolverOptions.CreateResolver()
			if err != nil {
				return fmt.Errorf("failed to create a UsesResolver: %w", err)
			}
		}
	}
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.QuietCommandRunner
	}
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}
	return nil
}

// Run implements this command
func (o *UsesOptions) Run() error {
	var err error
	err = o.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate options: %w", err)
	}

	migratorOwner := o.CatalogOwner
	migratorRepository := o.CatalogRepository

	rootDir := o.Dir
	if o.Catalog {
		if o.Processor == nil {
			o.Processor = processor.NewUsesMigrator(rootDir, o.TasksFolder, migratorOwner, migratorRepository, o.UseSHA, o.Catalog)
		}
		packsDir := filepath.Join(rootDir, "packs")
		err = filepath.Walk(packsDir, func(path string, info os.FileInfo, err error) error {
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
		o.Processor = processor.NewUsesMigrator(rootDir, o.TasksFolder, migratorOwner, migratorRepository, o.UseSHA, o.Catalog)
	}
	dir := filepath.Join(rootDir, ".lighthouse")
	exists, err := files.DirExists(dir)
	if err != nil {
		return fmt.Errorf("failed to check for dir %s: %w", dir, err)
	}
	if exists {
		err = o.ProcessDir(dir)
		if err != nil {
			return err
		}
		if o.TriggerCount > 0 {
			return nil
		}
	}

	// lets see if we have an old jenkins-x.yml file
	path := filepath.Join(rootDir, "jenkins-x.yml")
	exists, err = files.FileExists(path)
	if err != nil {
		return fmt.Errorf("failed to check for file %s: %w", path, err)
	}
	if !exists {
		log.Logger().Infof("no .lighthouse directories found")
		return nil
	}

	args := []string{"v2", "tekton", "converter"}
	if o.BatchMode {
		args = append(args, "--batch-mode")
	}
	if rootDir != "." && rootDir != "" {
		args = append(args, "--dir", rootDir)
	}
	c := &cmdrunner.Command{
		Dir:  rootDir,
		Name: "jx",
		Args: args,
		Out:  os.Stdout,
		Err:  os.Stderr,
		In:   os.Stdin,
	}
	_, err = o.CommandRunner(c)
	if err != nil {
		return fmt.Errorf("failed to run %s: %w", c.CLI(), err)
	}
	return nil
}

func (o *UsesOptions) ProcessDir(dir string) error {
	fs, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read dir %s: %w", dir, err)
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
			return fmt.Errorf("failed to check if file exists %s: %w", triggersFile, err)
		}
		if !exists {
			continue
		}

		triggers := &triggerconfig.Config{}
		err = yamls.LoadFile(triggersFile, triggers)
		if err != nil {
			return fmt.Errorf("failed to load lighthouse triggers: %s: %w", triggersFile, err)
		}

		o.TriggerCount++
		if !o.Catalog {
			o.Resolver, err = o.createNonCatalogResolver(triggerDir)
			if err != nil {
				return fmt.Errorf("failed to create resolver for non catalog in dir %s: %w", triggerDir, err)
			}
			if o.Resolver == nil {
				log.Logger().Infof("no Kptfile found in dir %s so cannot convert", info(triggerDir))
				continue
			}
		}
		err = o.processTriggerFile(triggers, triggerDir)
		if err != nil {
			return fmt.Errorf("failed to convert pipelines: %w", err)
		}
	}
	return nil
}

func (o *UsesOptions) processTriggerFile(repoConfig *triggerconfig.Config, dir string) error {
	modified := false
	for i := range repoConfig.Spec.Presubmits { //nolint:dupl
		r := &repoConfig.Spec.Presubmits[i]
		if r.SourcePath != "" {
			err := o.updateCatalogTask(r.SourcePath)
			if err != nil {
				return fmt.Errorf("failed to find catalog pipeline for %s: %w", r.SourcePath, err)
			}
			path := filepath.Join(dir, r.SourcePath)
			flag, err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				return fmt.Errorf("failed to convert %s: %w", r.SourcePath, err)
			}
			if flag {
				modified = true
			}
		}
		if r.Agent == "" && r.PipelineRunSpec != nil {
			r.Agent = job.TektonPipelineAgent
		}
	}
	for i := range repoConfig.Spec.Postsubmits { //nolint:dupl
		r := &repoConfig.Spec.Postsubmits[i]
		if r.SourcePath != "" {
			err := o.updateCatalogTask(r.SourcePath)
			if err != nil {
				return fmt.Errorf("failed to find catalog pipeline for %s: %w", r.SourcePath, err)
			}
			path := filepath.Join(dir, r.SourcePath)
			flag, err := processor.ProcessFile(o.Processor, path)
			if err != nil {
				return fmt.Errorf("failed to convert %s: %w", r.SourcePath, err)
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
			return fmt.Errorf("failed to check if file exists %s: %w", path, err)
		}
		if exists {
			err = gitclient.Remove(o.GitClient, dir, "Kptfile")
			if err != nil {
				return fmt.Errorf("failed to remove %s from git: %w", path, err)
			}
			err = os.RemoveAll(path)
			if err != nil {
				return fmt.Errorf("failed to remove file %s: %w", path, err)
			}
		}
	}
	return nil
}

func (o *UsesOptions) createNonCatalogResolver(triggerDir string) (*inrepo.UsesResolver, error) {
	path := filepath.Join(triggerDir, "Kptfile")
	exists, err := files.FileExists(path)
	if err != nil {
		return nil, fmt.Errorf("failed to check for file %s: %w", path, err)
	}
	if !exists {
		return nil, nil
	}

	// lets load the
	kf := &kptfile.KptFile{}
	err = yamls.LoadFile(path, kf)
	if err != nil {
		return nil, fmt.Errorf("failed to load the kptfile %s: %w", path, err)
	}

	// replace owner / repo / tag etc
	git := kf.Upstream.Git
	repoURL := git.Repo
	if repoURL == "" {
		return nil, fmt.Errorf("missing upstream.git.repo in %s", path)
	}
	gitInfo, err := giturl.ParseGitURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git URL %s from %s: %w", repoURL, path, err)
	}

	if gitInfo != nil {
		o.ResolverOptions.CatalogOwner = gitInfo.Organisation
		o.ResolverOptions.CatalogRepository = gitInfo.Name
		o.Processor.Owner = gitInfo.Organisation
		o.Processor.Repository = gitInfo.Name
	}

	resolver, err := o.ResolverOptions.CreateResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create the resolver: %w", err)
	}

	// optionally keep the kpt ref
	sha := "versionStream"
	if o.UseKptRef && git.Commit != "" {
		sha = git.Commit
	}

	// lets figure out the tasks folder from kpt
	tasksFolder := "tasks"
	paths := strings.Split(strings.TrimPrefix(git.Directory, "/"), string(os.PathSeparator))
	if len(paths) > 1 && paths[0] == "packs" {
		tasksFolder = filepath.Join(o.TasksFolder, paths[1])
	}

	o.Processor.TasksFolder = tasksFolder
	o.Processor.SHA = sha
	resolver.SHA = sha
	resolver.Dir = git.Directory
	return resolver, nil
}

// updateCatalogTask lets find the catalog task for the given file so that we can use it
func (o *UsesOptions) updateCatalogTask(sourceFile string) error {
	if o.Catalog {
		return nil
	}
	var err error
	if o.CatalogSHA == "" {
		o.CatalogSHA = o.Processor.SHA
	}
	o.Processor.CatalogTaskSpec, err = lighthouses.FindCatalogTaskSpec(o.Resolver, sourceFile, o.CatalogSHA)
	return err
}
