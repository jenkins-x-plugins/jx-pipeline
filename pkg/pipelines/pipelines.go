package pipelines

import (
	"strings"

	v1 "github.com/jenkins-x/jx-api/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/pkg/kube/activities"
	"github.com/jenkins-x/jx-helpers/pkg/kube/naming"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ownerLabels   = []string{"owner", "lighthouse.jenkins-x.io/refs.org"}
	repoLabels    = []string{"repository", "lighthouse.jenkins-x.io/refs.repo"}
	branchLabels  = []string{"branch", "lighthouse.jenkins-x.io/branch"}
	buildLabels   = []string{"build", "lighthouse.jenkins-x.io/buildNum"}
	contextLabels = []string{"context", "lighthouse.jenkins-x.io/context"}
)

func label(m map[string]string, labels []string) string {
	if m == nil {
		return ""
	}
	for _, l := range labels {
		value := m[l]
		if value != "" {
			return value
		}
	}
	return ""
}

func ToPipelineActivityName(pr *v1beta1.PipelineRun, paList []v1.PipelineActivity) string {
	labels := pr.Labels
	if labels == nil {
		return pr.Name
	}

	build := labels["build"]
	owner := label(labels, ownerLabels)
	repository := label(labels, repoLabels)
	branch := label(labels, branchLabels)

	if build == "" {
		buildID := labels["lighthouse.jenkins-x.io/buildNum"]
		if buildID == "" {
			return ""
		}
		found := false
		for i := range paList {
			pa := &paList[i]
			if pa.Labels == nil {
				continue
			}
			if pa.Labels["buildID"] == buildID {
				if pa.Spec.Build != "" {
					pr.Labels["build"] = pa.Spec.Build
					build = pa.Spec.Build
				}
				found = true
			}
		}
		if !found && owner != "" && repository != "" && branch != "" {
			build = "1"
			pr.Labels["build"] = build
		}
	}
	if owner != "" && repository != "" && branch != "" && build != "" {
		return naming.ToValidName(owner + "-" + repository + "-" + branch + "-" + build)
	}
	return ""
}

func ToPipelineActivity(pr *v1beta1.PipelineRun, pa *v1.PipelineActivity) {
	annotations := pr.Annotations
	labels := pr.Labels
	if pa.APIVersion == "" {
		pa.APIVersion = "jenkins.io/v1"
	}
	if pa.Kind == "" {
		pa.Kind = "PipelineActivity"
	}
	pa.Name = pr.Name
	pa.Namespace = pr.Namespace

	if pa.Annotations == nil {
		pa.Annotations = map[string]string{}
	}
	if pa.Labels == nil {
		pa.Labels = map[string]string{}
	}
	for k, v := range annotations {
		pa.Annotations[k] = v
	}
	for k, v := range labels {
		pa.Labels[k] = v
	}

	ps := &pa.Spec
	if labels != nil {
		if ps.GitOwner == "" {
			ps.GitOwner = label(labels, ownerLabels)
		}
		if ps.GitRepository == "" {
			ps.GitRepository = label(labels, repoLabels)
		}
		if ps.GitBranch == "" {
			ps.GitBranch = label(labels, branchLabels)
		}
		if ps.Build == "" {
			ps.Build = label(labels, buildLabels)
		}
		if ps.Context == "" {
			ps.Context = label(labels, contextLabels)
		}
		if ps.BaseSHA == "" {
			ps.BaseSHA = labels["lighthouse.jenkins-x.io/baseSHA"]
		}
		if ps.LastCommitSHA == "" {
			ps.LastCommitSHA = labels["lighthouse.jenkins-x.io/lastCommitSHA"]
		}
	}
	if annotations != nil {
		if ps.GitURL == "" {
			ps.GitURL = annotations["lighthouse.jenkins-x.io/cloneURI"]
		}
	}

	podName := ""
	var steps []v1.PipelineActivityStep
	if pr.Status.TaskRuns != nil {
		for _, v := range pr.Status.TaskRuns {
			if v.Status == nil {
				continue
			}
			if podName == "" {
				podName = v.Status.PodName
			}

			previousStepTerminated := false
			for _, step := range v.Status.Steps {
				name := step.Name
				var started *metav1.Time
				var completed *metav1.Time
				status := v1.ActivityStatusTypePending

				terminated := step.Terminated
				if terminated != nil {
					if terminated.ExitCode == 0 {
						status = v1.ActivityStatusTypeSucceeded
					} else if !terminated.FinishedAt.IsZero() {
						status = v1.ActivityStatusTypeFailed
					}
					started = &terminated.StartedAt
					completed = &terminated.FinishedAt
					previousStepTerminated = true
				} else if step.Running != nil {
					if previousStepTerminated {
						started = &step.Running.StartedAt
						status = v1.ActivityStatusTypeRunning
					}
					previousStepTerminated = false
				}

				paStep := v1.PipelineActivityStep{
					Kind: v1.ActivityStepKindTypeStage,
					Stage: &v1.StageActivityStep{
						CoreActivityStep: v1.CoreActivityStep{
							Name:               Humanize(name),
							Description:        "",
							Status:             status,
							StartedTimestamp:   started,
							CompletedTimestamp: completed,
						},
					},
				}
				steps = append(steps, paStep)
			}
		}
	}

	// if the PipelineActivity has some real steps lets trust it; otherise lets merge any prevew/promote steps
	// with steps from the PipelineRun
	// lets add any missing steps from the PipelineActivity as they may have been created via a `jx promote` step
	hasStep := false
	for _, s := range ps.Steps {
		if s.Kind == v1.ActivityStepKindTypeStage && s.Stage != nil && s.Stage.Name != "Release" {
			hasStep = true
			break
		}
	}
	if !hasStep {
		for _, s := range ps.Steps {
			if s.Kind == v1.ActivityStepKindTypePreview || s.Kind == v1.ActivityStepKindTypePromote {
				steps = append(steps, s)
			}
		}
		ps.Steps = steps
	}

	if len(ps.Steps) == 0 {
		ps.Steps = append(ps.Steps, v1.PipelineActivityStep{
			Kind: v1.ActivityStepKindTypeStage,
			Stage: &v1.StageActivityStep{
				CoreActivityStep: v1.CoreActivityStep{
					Name:   "initialising",
					Status: v1.ActivityStatusTypeRunning,
				},
			},
		})
	}

	if podName != "" {
		pa.Labels["podName"] = podName
	}

	activities.UpdateStatus(pa, false, nil)
}

// Humanize splits into words and capitalises
func Humanize(text string) string {
	wordsText := strings.ReplaceAll(strings.ReplaceAll(text, "-", " "), "_", " ")
	words := strings.Split(wordsText, " ")
	for i := range words {
		words[i] = strings.Title(words[i])
	}
	return strings.Join(words, " ")
}
