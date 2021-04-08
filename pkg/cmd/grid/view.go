package grid

import (
	"fmt"
	"strings"

	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-helpers/v3/pkg/table"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
)

func (m model) View() string {
	m.activityTable.lock.Lock()
	defer m.activityTable.lock.Unlock()

	if m.activityTable.stopped {
		return fmt.Sprintf("\npress the %s to go back to the pipeline grid or %s to quit\n\n", info("space bar"), info("q"))
	}

	s := &strings.Builder{}
	t := table.CreateTable(s)
	t.AddRow("REPOSITORY", "BRANCH", "BUILD", "CONTEXT", "STATUS", "LAST STEP")

	for i, name := range m.activityTable.names {
		if i >= m.activityTable.height {
			break
		}
		act := m.activityTable.index[name]
		if act == nil {
			continue
		}

		as := &act.Spec

		repo := as.GitOwner + "/" + as.GitRepository
		if i == m.activityTable.current {
			repo = termcolor.ColorStatus(repo)
		}
		t.AddRow(repo, as.GitBranch, as.Build, as.Context, ToPipelineStatus(as.Status), ToLastStep(act))
	}

	t.Render()
	return s.String()
}

func ToPipelineStatus(statusType v1.ActivityStatusType) string {
	text := statusType.String()
	switch statusType {
	case v1.ActivityStatusTypeFailed, v1.ActivityStatusTypeError:
		return termcolor.ColorError(text)
	case v1.ActivityStatusTypeSucceeded:
		return termcolor.ColorInfo(text)
	case v1.ActivityStatusTypeRunning:
		return termcolor.ColorStatus(text)
	default:
		return text
	}
}

func ToLastStep(pa *v1.PipelineActivity) string {
	s := &pa.Spec
	steps := s.Steps
	if len(steps) > 0 {
		step := steps[len(steps)-1]
		st := step.Stage
		if st != nil {
			ssteps := st.Steps
			for i := len(ssteps) - 1; i >= 0; i-- {
				ss := ssteps[i]
				if ss.Status == v1.ActivityStatusTypePending && i > 0 {
					continue
				}
				return ss.Name
			}
			return st.Name
		}
		promote := step.Promote
		if promote != nil {
			pr := promote.PullRequest
			prURL := ""
			if pr != nil {
				prURL = pr.PullRequestURL
			}
			if prURL != "" {
				title := ""
				//Todo: Can be simplified
				if pr != nil {
					title = pr.Name
				}
				if promote.Environment != "" {
					title = fmt.Sprintf("Promote to %s", strings.Title(promote.Environment))
				}
				return fmt.Sprintf(`%s %s`, title, prURL)
			}
			return promote.Name
		}
		preview := step.Preview
		if preview != nil {
			if preview.ApplicationURL != "" {
				title := preview.Name
				if title == "" {
					title = "Preview"
				}
				return fmt.Sprintf(`Promote %s %s`, title, preview.ApplicationURL)
			}
			return preview.Name
		}
		return st.Name
	}
	return ""
}
