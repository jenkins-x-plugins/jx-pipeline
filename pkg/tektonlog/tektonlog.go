package tektonlog

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/cloud/buckets"
	"github.com/jenkins-x-plugins/jx-pipeline/pkg/pipelines"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned"
	typev1 "github.com/jenkins-x/jx-api/v4/pkg/client/clientset/versioned/typed/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/pods"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/pkg/errors"
	tektonapis "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	tektonclient "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	info = termcolor.ColorInfo
)

// TektonLogger contains the necessary clients and the namespace to get data from the cluster, an implementation of
// LogWriter to write logs to and a logs retriever function to override the default way to obtain logs
type TektonLogger struct {
	JXClient           versioned.Interface
	TektonClient       tektonclient.Interface
	KubeClient         kubernetes.Interface
	Namespace          string
	GitUsername        string
	GitToken           string
	BytesLimit         int64
	FailIfPodFails     bool
	StorageReadTimeout time.Duration
	LogsRetrieverFunc  retrieverFunc
	err                error
}

// Err returns the last error that occurred during streaming logs.
// It should be checked after the log stream channel has been closed.
func (t *TektonLogger) Err() error {
	return t.err
}

// retrieverFunc is a func signature used to define the LogsRetrieverFunc in TektonLogger
type retrieverFunc func(ctx context.Context, pod *corev1.Pod, container *corev1.Container, limitBytes int64, c kubernetes.Interface) (io.ReadCloser, error)

// LogLine is the object sent to and received from the channels in the StreamLog and WriteLog functions
// defined by LogWriter
type LogLine struct {
	Line       string
	ShouldMask bool
}

func (t *TektonLogger) GetLogsForActivity(ctx context.Context, out io.Writer, pa *v1.PipelineActivity, name string, prList []*tektonapis.PipelineRun) error {
	if pa.Spec.BuildLogsURL != "" && pa.Spec.Status != v1.ActivityStatusTypeRunning {
		for line := range t.StreamPipelinePersistentLogs(pa.Spec.BuildLogsURL) {
			fmt.Fprintln(out, line.Line)
		}
		return t.Err()
	}

	log.Logger().Infof("Build logs for %s", termcolor.ColorInfo(name))
	name = strings.TrimSuffix(name, " ")

	for line := range t.GetRunningBuildLogs(ctx, pa, prList, name) {
		fmt.Fprintln(out, line.Line)
	}
	return t.Err()
}

// GetTektonPipelinesWithActivePipelineActivity returns list of all PipelineActivities with corresponding Tekton PipelineRuns ordered by the PipelineRun creation timestamp and a map to obtain its reference once a name has been selected
func (t *TektonLogger) GetTektonPipelinesWithActivePipelineActivity(ctx context.Context, filter *BuildPodInfoFilter) ([]string, map[string]*v1.PipelineActivity, map[string][]*tektonapis.PipelineRun, error) {
	paList, err := t.JXClient.JenkinsV1().PipelineActivities(t.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, nil, errors.Wrap(err, "there was a problem getting the PipelineActivities")
	}

	paNameMap := make(map[string]*v1.PipelineActivity)
	for i := range paList.Items {
		p := &paList.Items[i]
		paNameMap[p.Name] = p
	}

	tektonPRs, _ := t.TektonClient.TektonV1beta1().PipelineRuns(t.Namespace).List(ctx, metav1.ListOptions{})
	log.Logger().Debugf("found %d PipelineRuns in namespace %s", len(tektonPRs.Items), t.Namespace)

	prMap := make(map[string][]*tektonapis.PipelineRun)
	for i := range tektonPRs.Items {
		p := &tektonPRs.Items[i]
		paName := pipelines.ToPipelineActivityName(p, paList.Items)
		if paName == "" {
			continue
		}
		pa := paNameMap[paName]
		if pa == nil {
			pa = &v1.PipelineActivity{}
			pa.Name = paName
			paNameMap[paName] = pa
		}
		pipelines.ToPipelineActivity(p, pa, false)

		fullBuildName := createPipelineActivityName(pa)
		prMap[fullBuildName] = append(prMap[fullBuildName], p)
	}

	// lets make a sorted list of activities and use that...
	var sortedPA []*v1.PipelineActivity
	for _, pa := range paNameMap {
		sortedPA = append(sortedPA, pa)
	}
	sort.Slice(sortedPA, func(i, j int) bool {
		return sortedPA[i].CreationTimestamp.After(sortedPA[j].CreationTimestamp.Time)
	})

	paMap := make(map[string]*v1.PipelineActivity)
	for _, p := range paNameMap {
		paMap[createPipelineActivityName(p)] = p
	}

	var names []string
	for _, pa := range sortedPA {
		if !filter.Matches(pa) {
			continue
		}
		paName := createPipelineActivityName(pa)
		if _, exists := prMap[paName]; exists {
			hasNonPendingPR := false
			for _, pr := range prMap[paName] {
				if PipelineRunIsNotPending(pr) {
					hasNonPendingPR = true
				}
			}
			if hasNonPendingPR {
				names = append(names, paName)
			}
		} else if pa.Spec.CompletedTimestamp != nil {
			names = append(names, paName)
		}
	}

	return names, paMap, prMap, nil
}

// GetPipelineActivityForPipelineRun returns the PipelineActivity for the PipelineRun if it can be found
func GetPipelineActivityForPipelineRun(ctx context.Context, activityInterface typev1.PipelineActivityInterface, pr *tektonapis.PipelineRun) (*v1.PipelineActivity, error) {
	resources, err := activityInterface.List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to load PipelineActivity resources")
	}
	var paList []v1.PipelineActivity
	if resources != nil {
		paList = resources.Items
	}
	name := pipelines.ToPipelineActivityName(pr, paList)
	if name == "" {
		return nil, nil
	}
	pa, err := activityInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return pa, errors.Wrapf(err, "failed to find PipelineActivity %s", name)
	}
	return pa, nil
}

func createPipelineActivityName(pa *v1.PipelineActivity) string {
	ps := &pa.Spec
	buildNumber := ps.Build
	owner := ps.GitOwner
	repository := ps.GitRepository
	branch := ps.GitBranch
	triggerContext := ps.Context

	baseName := fmt.Sprintf("%s/%s/%s #%s",
		naming.ToValidName(owner),
		naming.ToValidName(repository),
		naming.ToValidName(branch),
		strings.ToLower(buildNumber))

	if triggerContext != "" {
		return fmt.Sprintf("%s %s", baseName, naming.ToValidName(triggerContext))
	}
	return baseName
}

// GetRunningBuildLogs obtains the logs of the provided PipelineActivity and streams the running build pods' logs using the provided LogWriter
func (t *TektonLogger) GetRunningBuildLogs(ctx context.Context, pa *v1.PipelineActivity, pipelineRuns []*tektonapis.PipelineRun, buildName string) <-chan LogLine {
	ch := make(chan LogLine)
	go func() {
		defer close(ch)
		err := t.getRunningBuildLogs(ctx, pa, pipelineRuns, buildName, ch)
		if err != nil {
			t.err = err
		}
	}()
	return ch
}

type stageTime struct {
	name      string
	startTime *metav1.Time
	task      string
	skipped   bool
	podExists bool
}

func (t *TektonLogger) getRunningBuildLogs(ctx context.Context, pa *v1.PipelineActivity, pipelineRuns []*tektonapis.PipelineRun, buildName string, out chan<- LogLine) error {
	loggedAllRunsForActivity := false
	foundLogs := false
	completedStages := map[string]bool{}

	// Make sure we check again for the build pipeline if we just get the metapipeline initially, assuming the metapipeline succeeds
	for !loggedAllRunsForActivity {
		var stages = t.collectStages(ctx, pipelineRuns)
		for _, stage := range stages {
			podName := stage.name
			stageName := stage.task
			if completedStages[stageName] {
				continue
			}
			if stage.podExists {
				log.Logger().Infof("logging pod: %s for task %s", info(podName), stageName)

				pod, err := t.KubeClient.CoreV1().Pods(t.Namespace).Get(ctx, podName, metav1.GetOptions{})
				if err != nil && apierrors.IsNotFound(err) {
					if pa.Spec.Status == v1.ActivityStatusTypeRunning {
						pa.Spec.Status = v1.ActivityStatusTypeAborted
					}
					return nil
				}
				if err != nil {
					return errors.Wrapf(err, "failed to load pod %s in namespace %s", podName, t.Namespace)
				}

				err = t.getContainerLogsFromPod(ctx, pod, pa, buildName, stageName, out)
				if err != nil {
					return errors.Wrapf(err, "failed to get logs for pod %s", podName)
				}

				err = pods.WaitForPodNameToBeComplete(t.KubeClient, t.Namespace, podName, 1*time.Second)
				if err == nil {
					completedStages[podName] = true
				}

				if pods.IsPodCompleted(pod) {
					completedStages[stageName] = true
				}
			} else if stage.skipped {
				completedStages[stageName] = true
				log.Logger().Infof("pod is skipped/failed for task: %s", stageName)
			} else {
				// let's wait second for next pod/task start
				time.Sleep(time.Second)
				log.Logger().Debugf("no pod found for task: %s", stageName)
			}
		}

		if len(completedStages) > 0 {
			foundLogs = true
		}

		// if all pods completed lets move out from the loop
		if len(completedStages) == len(stages) {
			loggedAllRunsForActivity = true
		}
	}
	if !foundLogs {
		return errors.New("the build pods for this build have been garbage collected and the log was not found in the long term storage bucket")
	}
	return nil
}

func (t *TektonLogger) collectStages(ctx context.Context, pipelineRuns []*tektonapis.PipelineRun) []stageTime {
	var podTimes []stageTime
	for _, prInitial := range pipelineRuns {
		//we need fresh pipeline to be able consume newly executed tasks/pods
		pr, _ := t.TektonClient.TektonV1beta1().PipelineRuns(t.Namespace).Get(ctx, prInitial.Name, metav1.GetOptions{})
		for _, taskStatus := range pr.Status.PipelineSpec.Tasks {
			podTime := findExecutedOrSkippedStagesStage(taskStatus.Name, pr)
			podTimes = append(podTimes, podTime)
		}
	}
	sort.Slice(podTimes, func(i, j int) bool {
		t1 := podTimes[i].startTime
		t2 := podTimes[j].startTime
		if t1 == nil && t2 == nil {
			return false
		}
		if t1 == nil {
			return true
		}
		if t2 == nil {
			return false
		}
		return t1.Before(t2)
	})
	return podTimes
}

func findExecutedOrSkippedStagesStage(taskName string, pr *tektonapis.PipelineRun) stageTime {
	for _, taskStatus := range pr.Status.TaskRuns {
		if taskName == taskStatus.PipelineTaskName && taskStatus.Status.PodName != "" {
			return stageTime{
				name:      taskStatus.Status.PodName,
				startTime: taskStatus.Status.StartTime,
				task:      taskName,
				podExists: true,
			}
		}
	}
	for _, taskStatus := range pr.Status.SkippedTasks {
		if taskName == taskStatus.Name {
			return stageTime{
				skipped: true,
				task:    taskName,
			}
		}
	}
	return stageTime{
		task: taskName,
	}
}

func (t *TektonLogger) getContainerLogsFromPod(ctx context.Context, pod *corev1.Pod, pa *v1.PipelineActivity, buildName, stageName string, out chan<- LogLine) error {
	infoColor := color.New(color.FgGreen)
	infoColor.EnableColor()
	errorColor := color.New(color.FgRed)
	errorColor.EnableColor()
	containers, _, _ := pods.GetContainersWithStatusAndIsInit(pod)
	for i := range containers {
		ic := &containers[i]
		var err error
		pod, err = t.waitForContainerToStart(ctx, pa.Namespace, pod, i, stageName, out)
		out <- LogLine{
			Line: fmt.Sprintf("\nShowing logs for build %v stage %s and container %s",
				infoColor.Sprintf(buildName), infoColor.Sprintf(stageName), infoColor.Sprintf(ic.Name)),
		}
		if err != nil {
			return errors.Wrapf(err, "there was a problem writing a single line into the logs writer")
		}
		err = t.fetchLogsToChannel(ctx, pod, ic, out)
		if err != nil {
			return errors.Wrap(err, "couldn't fetch logs into the logs channel")
		}
		if hasStepFailed(ctx, pod, i, t.KubeClient, pa.Namespace) {
			out <- LogLine{
				Line: errorColor.Sprintf("\nPipeline failed on stage '%s' : container '%s'. The execution of the pipeline has stopped.", stageName, ic.Name),
			}
			if t.FailIfPodFails {
				return errors.Errorf("Pipeline failed on stage '%s' : container '%s'. The execution of the pipeline has stopped.", stageName, ic.Name)
			}
			break
		}
	}
	return nil
}

func (t *TektonLogger) fetchLogsToChannel(ctx context.Context, pod *corev1.Pod, container *corev1.Container, out chan<- LogLine) error {
	logsRetrieverFunc := t.LogsRetrieverFunc
	if logsRetrieverFunc == nil {
		logsRetrieverFunc = retrieveLogsFromPod
	}
	reader, err := logsRetrieverFunc(ctx, pod, container, t.BytesLimit, t.KubeClient)
	if err != nil {
		return err
	}
	defer reader.Close()
	return writeStreamLines(reader, out)
}

func writeStreamLines(reader io.Reader, out chan<- LogLine) error {
	buffReader := bufio.NewReader(reader)
	for {
		line, _, err := buffReader.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return errors.Wrap(err, "failed to read stream")
		}
		out <- LogLine{Line: string(line), ShouldMask: true}
	}
}

func hasStepFailed(ctx context.Context, pod *corev1.Pod, stepNumber int, kubeClient kubernetes.Interface, ns string) bool {
	pod, err := kubeClient.CoreV1().Pods(ns).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		log.Logger().Error("couldn't find the updated pod to check the step status")
		return false
	}
	_, containerStatus, _ := pods.GetContainersWithStatusAndIsInit(pod)
	if containerStatus[stepNumber].State.Terminated != nil && containerStatus[stepNumber].State.Terminated.ExitCode != 0 {
		return true
	}
	return false
}

func (t *TektonLogger) waitForContainerToStart(ctx context.Context, ns string, pod *corev1.Pod, idx int, stageName string, out chan<- LogLine) (*corev1.Pod, error) {
	if pod.Status.Phase == corev1.PodFailed {
		return pod, nil
	}
	if pods.HasContainerStarted(pod, idx) {
		return pod, nil
	}
	containerName := ""
	containers, _, _ := pods.GetContainersWithStatusAndIsInit(pod)
	if idx < len(containers) {
		containerName = containers[idx].Name
	}
	// This method will be executed by both the CLI and the UI, we don't know if the UI has color enabled, so we are using a local instance instead of the global one
	c := color.New(color.FgGreen)
	c.EnableColor()
	out <- LogLine{
		Line: fmt.Sprintf("\nwaiting for stage %s : container %s to start...\n", c.Sprintf(stageName), c.Sprintf(containerName)),
	}
	for {
		time.Sleep(time.Second)
		p, err := t.KubeClient.CoreV1().Pods(ns).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return p, errors.Wrapf(err, "failed to load pod %s", pod.Name)
		}
		if pods.HasContainerStarted(p, idx) {
			return p, nil
		}
	}
}

// StreamPipelinePersistentLogs reads logs from the provided bucket URL and writes them using the provided LogWriter
func (t *TektonLogger) StreamPipelinePersistentLogs(logsURL string) <-chan LogLine {
	ch := make(chan LogLine)
	go func() {
		defer close(ch)
		err := t.streamPipelinePersistentLogs(logsURL, ch)
		if err != nil {
			t.err = err
		}
	}()
	return ch
}

func (t *TektonLogger) streamPipelinePersistentLogs(logsURL string, out chan<- LogLine) error {
	if t.StorageReadTimeout.Nanoseconds() == 0 {
		t.StorageReadTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.StorageReadTimeout)
	defer cancel()

	reader, err := buckets.ReadURL(ctx, logsURL, 30*time.Second, t.CreateBucketHTTPFn())
	if err != nil {
		return errors.Wrapf(err, "there was a problem obtaining the log file from the github pages URL %s", logsURL)
	}
	return t.streamPipedLogs(reader, out)
}

func (t *TektonLogger) streamPipedLogs(src io.ReadCloser, out chan<- LogLine) (err error) {
	defer func() {
		if e := src.Close(); e != nil && err == nil {
			err = e
		}
	}()
	scanner := bufio.NewScanner(src)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		text := scanner.Text()
		out <- LogLine{Line: text}
		if t.FailIfPodFails && strings.Contains(text, "The execution of the pipeline has stopped.") {
			return errors.New("the execution of the pipeline has stopped")
		}
	}
	return nil
}

// Uses the same signature as retrieverFunc so it can be used in TektonLogger
func retrieveLogsFromPod(ctx context.Context, pod *corev1.Pod, container *corev1.Container, limitBytes int64, client kubernetes.Interface) (io.ReadCloser, error) {
	options := &corev1.PodLogOptions{
		Container: container.Name,
		Follow:    true,
	}
	if limitBytes > 0 {
		options.LimitBytes = &limitBytes
	}
	req := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, options)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "there was an error creating the logs stream for pod %s", pod.Name)
	}
	return stream, nil
}
