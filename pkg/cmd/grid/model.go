package grid

import (
	tea "github.com/charmbracelet/bubbletea"
	v1 "github.com/jenkins-x/jx-api/v4/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"sort"
	"strings"
	"sync"
)

type model struct {
	activityTable *activityTable
	sub           chan struct{} // where we'll receive activity notifications
	current       int
	filter        string
}

type activityTable struct {
	lock    sync.Mutex
	max     int
	height  int
	stopped bool
	names   []string
	index   map[string]*v1.PipelineActivity
}

func (a *activityTable) reindex() {
	var names []string
	for k := range a.index {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		a1 := a.index[names[i]]
		a2 := a.index[names[j]]
		return a1.CreationTimestamp.After(a2.CreationTimestamp.Time)
	})
	a.names = names

	a.max = len(names)
	if a.max > a.height {
		a.max = a.height
	}
}

func newModel(filter string) model {
	return model{
		activityTable: &activityTable{
			index:  map[string]*v1.PipelineActivity{},
			height: 10,
		},
		filter: filter,
		sub:    make(chan struct{}),
	}
}

// A message used to indicate that activity has occurred. In the real world (for
// example, chat) this would contain actual data.
type responseMsg struct{}

// A command that waits for the activity on a channel.
func waitForActivity(sub chan struct{}) tea.Cmd {
	return func() tea.Msg {
		return responseMsg(<-sub)
	}
}

func (m model) Init() tea.Cmd {
	return waitForActivity(m.sub)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.activityTable.height = msg.Height - 2

	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c", "q":
			m.stop()
			return m, tea.Quit

		case "enter":
			m.stop()
			return m, tea.Quit

		case "down", "j":
			if m.current+1 < m.activityTable.max {
				m.current++
			}
			return m, nil

		case "up", "k":
			if m.current > 0 {
				m.current--
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
		return m, waitForActivity(m.sub)
	}

	log.Logger().Infof("unknown event %#v", msg)
	return m, nil
}

func (m model) onPipelineActivity(a *v1.PipelineActivity) {
	if m.filter != "" && !strings.Contains(a.Name, m.filter) {
		return
	}

	m.activityTable.lock.Lock()

	m.activityTable.index[a.Name] = a
	m.activityTable.reindex()

	m.activityTable.lock.Unlock()

	m.sub <- struct{}{}
}

func (m model) deletePipelineActivity(name string) {
	m.activityTable.lock.Lock()

	delete(m.activityTable.index, name)
	m.activityTable.reindex()

	m.activityTable.lock.Unlock()

	m.sub <- struct{}{}
}

func (m model) count() int {
	m.activityTable.lock.Lock()
	defer m.activityTable.lock.Unlock()

	return len(m.activityTable.names)
}

func (m model) stop() {
	m.activityTable.stopped = true
	// avoid waiting forever
	m.sub <- struct{}{}
}
