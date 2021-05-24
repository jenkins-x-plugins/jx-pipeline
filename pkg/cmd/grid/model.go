package grid

import (
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/activities"
	"sort"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
)

type model struct {
	activityTable *activityTable
	ch            chan struct{}
	filter        string
}

type activityTable struct {
	lock       sync.Mutex
	current    int
	max        int
	height     int
	stopped    bool
	names      []string
	index      map[string]*v1.PipelineActivity
	viewLogsFn func(act *v1.PipelineActivity, paList []v1.PipelineActivity) error
}

func (a *activityTable) selected() *v1.PipelineActivity {
	c := a.current
	if c >= a.max || c >= len(a.names) {
		return nil
	}
	return a.index[a.names[c]]
}

func (a *activityTable) reindex() {
	var names []string
	for k := range a.index {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		a1 := a.index[names[i]]
		a2 := a.index[names[j]]
		s1 := a1.Spec.StartedTimestamp
		s2 := a2.Spec.StartedTimestamp
		if s1 != nil && s2 != nil {
			if s1.Time != s2.Time {
				return s1.After(s2.Time)
			}
			diff := strings.Compare(a2.Spec.GitOwner, a1.Spec.GitOwner)
			if diff != 0 {
				return diff < 0
			}
			diff = strings.Compare(a2.Spec.GitRepository, a1.Spec.GitRepository)
			if diff != 0 {
				return diff < 0
			}
			diff = strings.Compare(a2.Spec.GitBranch, a1.Spec.GitBranch)
			if diff != 0 {
				return diff < 0
			}
			diff = strings.Compare(a2.Spec.Context, a1.Spec.Context)
			if diff != 0 {
				return diff < 0
			}
			diff = buildNumber(a2) - buildNumber(a1)
			if diff != 0 {
				return diff < 0
			}
		}
		return a1.CreationTimestamp.After(a2.CreationTimestamp.Time)
	})
	a.names = names

	a.max = len(names)
	if a.max > a.height {
		a.max = a.height
	}
}

func buildNumber(a *v1.PipelineActivity) int {
	text := a.Spec.Build
	if text == "" {
		return 0
	}
	answer, _ := strconv.Atoi(text)
	return answer
}

func (a *activityTable) activityList() []v1.PipelineActivity {
	a.lock.Lock()
	defer a.lock.Unlock()

	var answer []v1.PipelineActivity
	for _, v := range a.index {
		answer = append(answer, *v)
	}
	return answer
}

func (a *activityTable) viewLogs() {
	act := a.selected()
	a.stopped = true
	if act != nil {
		a.viewLogsFn(act, a.activityList())
	}
}

func newModel(filter string, viewLogsFn func(act *v1.PipelineActivity, paList []v1.PipelineActivity) error) model {
	return model{
		activityTable: &activityTable{
			index:      map[string]*v1.PipelineActivity{},
			height:     10,
			viewLogsFn: viewLogsFn,
		},
		filter: filter,
		ch:     make(chan struct{}),
	}
}

// responseMsg used to indicate that activity from k8s has occurred
type responseMsg struct{}

// A command that waits for the activity on a channel.
func waitForActivity(sub chan struct{}) tea.Cmd {
	return func() tea.Msg {
		return responseMsg(<-sub)
	}
}

func (m model) Init() tea.Cmd {
	return waitForActivity(m.ch)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.activityTable.height = msg.Height - 2
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {

		case "space", " ", "s":
			log.Logger().Infof("viewing the grid again...")
			m.activityTable.stopped = false
			m.ch <- struct{}{}
			return m, waitForActivity(m.ch)

		case "ctrl+c", "q":
			m.stop()
			return m, tea.Quit

		case "enter":
			m.activityTable.viewLogs()
			return m, waitForActivity(m.ch)

		case "down", "j":
			if m.activityTable.current+1 < m.activityTable.max {
				m.activityTable.current++
			}
			return m, nil

		case "up", "k":
			if m.activityTable.current > 0 {
				m.activityTable.current--
			}
			return m, nil

		default:
			log.Logger().Infof("unknown key %s", msg.String())
			return m, nil
		}

	case responseMsg:
		if m.activityTable.stopped {
			return m, tea.Quit
		}
		return m, waitForActivity(m.ch)
	}

	log.Logger().Infof("unknown event %#v", msg)
	return m, nil
}

func (m model) onPipelineActivity(a *v1.PipelineActivity) {
	if m.filter != "" && !strings.Contains(a.Name, m.filter) {
		return
	}

	activities.DefaultValues(a)

	m.activityTable.lock.Lock()

	m.activityTable.index[a.Name] = a
	m.activityTable.reindex()

	m.activityTable.lock.Unlock()

	if !m.activityTable.stopped {
		m.ch <- struct{}{}
	}
}

func (m model) deletePipelineActivity(name string) {
	m.activityTable.lock.Lock()

	delete(m.activityTable.index, name)
	m.activityTable.reindex()

	m.activityTable.lock.Unlock()

	if !m.activityTable.stopped {
		m.ch <- struct{}{}
	}
}

func (m model) stop() {
	m.activityTable.stopped = true
	// avoid waiting forever
	m.ch <- struct{}{}
}
