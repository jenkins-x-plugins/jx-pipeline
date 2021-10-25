package importcmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jenkins-x-plugins/jx-pipeline/pkg/plugins"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/v3/pkg/files"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/gitdiscovery"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input"
	"github.com/jenkins-x/jx-helpers/v3/pkg/input/inputfactory"
	"github.com/jenkins-x/jx-helpers/v3/pkg/options"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-helpers/v3/pkg/yamls"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/lighthouse-client/pkg/config/job"
	"github.com/jenkins-x/lighthouse-client/pkg/triggerconfig"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jenkins-x/jx-helpers/v3/pkg/cobras/templates"
)

// Options contains the command line options
type Options struct {
	options.BaseOptions

	Dir                string
	ToDir              string
	CatalogURL         string
	CatalogDir         string
	TaskFolder         string
	TaskVersion        string
	TaskFilter         string
	KptBinary          string
	ReleaseBranches    []string
	NoTrigger          bool
	GitClient          gitclient.Interface
	CommandRunner      cmdrunner.CommandRunner
	QuietCommandRunner cmdrunner.CommandRunner
	Input              input.Interface
}

var (
	info = termcolor.ColorInfo

	cmdLong = templates.LongDesc(`
		Imports tekton pipelines from a catalog.

`)

	cmdExample = templates.Examples(`
		# import tekton tasks from the tekton catalog: be prompted for what tasks/version to import an whether to enable triggers
		jx pipeline import

		# import tasks filtering the list of folders for those matching 'build' and disabling the automatic lighthouse trigger
		jx pipeline import -f build --no-trigger

	`)

	triggerPresubmit  = "presubmit: trigger the Task on Pull Requests"
	triggerPostsubmit = "postsubmit: trigger the Task on a Release due to merge to the main branch"

	triggers = []string{
		triggerPresubmit, triggerPostsubmit,
	}
)

// NewCmdPipelineImport creates the command
func NewCmdPipelineImport() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "import",
		Short:   "Imports tekton pipelines from a catalog",
		Long:    cmdLong,
		Example: cmdExample,
		Aliases: []string{"build", "run"},
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&o.NoTrigger, "no-trigger", "", false, "No lighthouse trigger to be added")

	cmd.Flags().StringVarP(&o.Dir, "dir", "d", ".", "The directory which will have the .lighthouse/$folder/*.yaml files created")
	cmd.Flags().StringVarP(&o.ToDir, "to-dir", "", "", "The directory inside .lighthouse to import the resources. Defaults to the task folder")
	cmd.Flags().StringVarP(&o.CatalogURL, "url", "u", "https://github.com/tektoncd/catalog.git", "The tekton catalog git URL which is cloned if no --catalog-dir is specified")
	cmd.Flags().StringVarP(&o.CatalogDir, "catalog-dir", "", "", "The directory containing the tekton catalog. Usually only used for testing")
	cmd.Flags().StringVarP(&o.TaskFolder, "task", "t", "", "The name of the folder in the 'tasks' directory in the catalog to import resources from. If not specified you will be prompted to choose one")
	cmd.Flags().StringVarP(&o.TaskFilter, "filter", "f", "", "Filter the list of task folders for all that contain this text")
	cmd.Flags().StringVarP(&o.TaskVersion, "version", "v", "", "The version of the task folder to use")
	cmd.Flags().StringVarP(&o.KptBinary, "bin", "", "", "the 'kpt' binary name to use. If not specified this command will download the jx binary plugin into ~/.jx3/plugins/bin and use that")

	cmd.Flags().StringArrayVarP(&o.ReleaseBranches, "release-branch", "", nil, "the release branch regular expressions to trigger the release pipeline. If not specified defaults to: 'master', 'main'")

	return cmd, o
}

// Validate verifies things are setup correctly
func (o *Options) Validate() error {
	if len(o.ReleaseBranches) == 0 {
		o.ReleaseBranches = []string{"main", "master"}
	}
	if o.Input == nil {
		o.Input = inputfactory.NewInput(&o.BaseOptions)
	}
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.DefaultCommandRunner
	}
	if o.KptBinary == "" {
		var err error
		o.KptBinary, err = plugins.GetKptBinary(plugins.KptVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to download the kpt binary")
		}
	}
	// lazy create
	o.Git()
	return nil
}

// Run implements this command
func (o *Options) Run() error {
	var err error
	var fs []os.FileInfo
	err = o.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate options")
	}

	lhDir := filepath.Join(o.Dir, ".lighthouse")
	err = os.MkdirAll(lhDir, files.DefaultDirWritePermissions)
	if err != nil {
		return errors.Wrapf(err, "failed to create lighthouse dir %s", lhDir)
	}

	if o.CatalogDir == "" {
		log.Logger().Infof("loading tekton tasks from: %s ...", info(o.CatalogURL))
		o.CatalogDir, err = gitclient.CloneToDir(o.GitClient, o.CatalogURL, "")
		if err != nil {
			return errors.Wrapf(err, "failed to clone tekton catalog %s", o.CatalogURL)
		}
	}
	if o.CatalogURL == "" {
		o.CatalogURL, err = gitdiscovery.FindGitURLFromDir(o.CatalogDir, false)
		if err != nil {
			return errors.Wrapf(err, "failed to discover git clone URL from dir %s", o.CatalogDir)
		}
		log.Logger().Infof("loading tekton tasks from catalog %s", info(o.CatalogURL))
	}
	log.Logger().Infof("")

	taskDir := filepath.Join(o.CatalogDir, "task")
	exists, err := files.DirExists(taskDir)
	if err != nil {
		return errors.Wrapf(err, "failed to check if the task dir exists %s", taskDir)
	}
	if !exists {
		return errors.Errorf("the catalog does not include a 'task' directory at %s", o.CatalogDir)
	}

	if o.TaskFolder == "" {
		var names []string
		fs, err = ioutil.ReadDir(taskDir)
		if err != nil {
			return errors.Wrapf(err, "failed to read task dir %s", taskDir)
		}
		for _, f := range fs {
			name := f.Name()
			if !f.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			if o.TaskFilter == "" || strings.Contains(name, o.TaskFilter) {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		defaultName := ""
		o.TaskFolder, err = o.Input.PickNameWithDefault(names, "Which task do you want to import: ", defaultName, "pick the name of the task folder to import tekton resources from")
		if err != nil {
			return err
		}
		if o.TaskFolder == "" {
			return errors.Errorf("no task folder chosen")
		}
	}

	versionFolder := filepath.Join(taskDir, o.TaskFolder)
	exists, err = files.DirExists(versionFolder)
	if err != nil {
		return errors.Wrapf(err, "failed to check if the task versions folder exists %s", versionFolder)
	}
	if !exists {
		return errors.Errorf("could not find the task versions folder: %s", versionFolder)
	}

	if o.TaskVersion == "" {
		var versions []string
		fs, err = ioutil.ReadDir(versionFolder)
		if err != nil {
			return errors.Wrapf(err, "failed to read task dir %s", versionFolder)
		}
		for _, f := range fs {
			name := f.Name()
			if !f.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			versions = append(versions, name)
		}
		sort.Strings(versions)

		defaultVersion := ""
		o.TaskVersion, err = o.Input.PickNameWithDefault(versions, "Which version do you want to import: ", defaultVersion, "pick the version of the task folder to import tekton resources from")
		if err != nil {
			return err
		}
		if o.TaskVersion == "" {
			return errors.Errorf("no task version chosen")
		}
	}

	folder := filepath.Join(versionFolder, o.TaskVersion)
	exists, err = files.DirExists(folder)
	if err != nil {
		return errors.Wrapf(err, "failed to check if the task version folder exists %s", folder)
	}
	if !exists {
		return errors.Errorf("could not find the task version folder: %s", folder)
	}

	var fileNames []string
	fs, err = ioutil.ReadDir(folder)
	if err != nil {
		return errors.Wrapf(err, "failed to read task dir %s", folder)
	}
	for _, f := range fs {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)

	if len(fileNames) == 0 {
		return errors.Errorf("task version folder %s has no *.yaml files", folder)
	}

	log.Logger().Infof("importing files %s from %s version %s", info(strings.Join(fileNames, " ")), info(o.TaskFolder), info(o.TaskVersion))

	if o.ToDir == "" {
		o.ToDir = o.TaskFolder
	}

	u := o.CatalogURL
	if !strings.HasSuffix(u, ".git") {
		u = strings.TrimSuffix(u, "/") + ".git"
	}
	path := filepath.Join(o.Dir, ".lighthouse", o.ToDir)

	folderExpression := u + "/" + filepath.Join("task", o.TaskFolder, o.TaskVersion)
	args := []string{"pkg", "get", folderExpression, path}
	c := &cmdrunner.Command{
		Name: o.KptBinary,
		Args: args,
		Dir:  o.Dir,
	}
	_, err = o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to import the tekton resources via kpt: %s", c.CLI())
	}

	log.Logger().Infof("tekton files imported to %s", info(path))

	if !o.NoTrigger {
		err = o.addLighthouseTriggers(fileNames, path)
		if err != nil {
			return errors.Wrapf(err, "failed to add lighthouse triggers")
		}
	}

	err = gitclient.Add(o.GitClient, o.Dir, ".lighthouse")
	if err != nil {
		return errors.Wrapf(err, "failed to add the .lighthouse files to git")
	}

	c = &cmdrunner.Command{
		Name: "git",
		Args: []string{"status"},
		Dir:  o.Dir,
		Out:  os.Stdout,
		Err:  os.Stderr,
	}
	_, err = o.CommandRunner(c)
	if err != nil {
		return errors.Wrapf(err, "failed to run: %s", c.CLI())
	}

	log.Logger().Infof("please review and commit the git changes")
	return nil
}

// addLighthouseTriggers if enabled lets add the lighthouse triggers
func (o *Options) addLighthouseTriggers(fileNames []string, toDir string) error {
	lhTriggers := &triggerconfig.Config{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "config.lighthouse.jenkins-x.io/v1alpha1",
			Kind:       "TriggerConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "triggers",
		},
	}

	for _, f := range fileNames {
		name := strings.TrimSuffix(f, ".yaml")

		selected, err := o.Input.SelectNames(triggers, "select which lighthouse triggers to enable for task: "+name, true, "do you wish to trigger the Task")
		if err != nil {
			return errors.Wrapf(err, "failed to select triggers to enable")
		}
		if len(selected) == 0 {
			return nil
		}

		for _, s := range selected {
			base := job.Base{
				Agent:      job.TektonPipelineAgent,
				Name:       name,
				SourcePath: f,
			}
			switch s {
			case triggerPresubmit:
				lhTriggers.Spec.Presubmits = []job.Presubmit{
					{
						Base: base,
						Reporter: job.Reporter{
							Context: name,
						},
						AlwaysRun:    true,
						RerunCommand: "/" + name,
						Trigger:      "(?m)^/" + name + "?(s+|$)",
					},
				}
			case triggerPostsubmit:
				lhTriggers.Spec.Postsubmits = []job.Postsubmit{
					{
						Base: base,
						Brancher: job.Brancher{
							Branches: o.ReleaseBranches,
						},
						Reporter: job.Reporter{
							Context: name,
						},
					},
				}
			default:
				return errors.Errorf("unknown trigger %s", s)
			}
		}
	}
	if len(lhTriggers.Spec.Postsubmits) > 0 || len(lhTriggers.Spec.Presubmits) > 0 {
		outFile := filepath.Join(toDir, "triggers.yaml")
		err := yamls.SaveFile(lhTriggers, outFile)
		if err != nil {
			return errors.Wrapf(err, "failed to save file %s", outFile)
		}
		log.Logger().Infof("created lighthouse triggers file %s", info(outFile))
	}
	return nil
}

// Git returns the git client lazy creating one if it does not exist
func (o *Options) Git() gitclient.Interface {
	if o.QuietCommandRunner == nil {
		o.QuietCommandRunner = cmdrunner.QuietCommandRunner
	}
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.QuietCommandRunner)
	}
	return o.GitClient
}
