package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("236"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("204"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
)

type Model struct {
	jobs      []JobView
	workers   []WorkerView
	logs      []string
	selected  int
	tick      int
	serverURL string
}

type JobView struct {
	ID       string
	Type     string
	Status   string
	Priority int
	Age      string
}

type WorkerView struct {
	ID     string
	Status string
	Active string
}

func NewModel() *Model {
	return &Model{
		jobs:      make([]JobView, 0),
		workers:   make([]WorkerView, 0),
		logs:      make([]string, 0),
		serverURL: "http://localhost:8080",
	}
}

func NewModelWithServer(url string) *Model {
	m := NewModel()
	m.serverURL = url
	return m
}

func (m *Model) Init() tea.Cmd {
	return fetchDataCmd(m.serverURL)
}

type TickMsg time.Time
type DataMsg struct {
	Jobs    []JobView
	Workers []WorkerView
}

func DataMsgFromState(state StateResponse) DataMsg {
	var dm DataMsg
	dm.Jobs = state.Jobs
	dm.Workers = state.Workers
	return dm
}

type ErrorMsg string

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case DataMsg:
		m.jobs = msg.Jobs
		m.workers = msg.Workers
		if m.selected < len(m.jobs) && m.selected >= 0 {
			m.logs = []string{fmt.Sprintf("Selected job: %s", m.jobs[m.selected].ID)}
		}
	case ErrorMsg:
		m.logs = []string{string(msg)}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			if m.selected > 0 {
				m.selected--
			}
		case "down":
			if m.selected < len(m.jobs)-1 {
				m.selected++
			}
		}
	case TickMsg:
		m.tick++
	}
	return m, tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m *Model) View() string {
	s := titleStyle.Render("GopherFlow Dashboard") + "\n"
	s += fmt.Sprintf("Tick: %d\n\n", m.tick)

	border := borderStyle.Width(60)
	border120 := borderStyle.Width(120)

	leftPanel := border.Render(m.jobsView())
	rightPanel := border.Render(m.workersView())

	s += lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		rightPanel,
	)

	s += "\n\n" + border120.Render(m.logsView())

	return s
}

func (m *Model) jobsView() string {
	s := infoStyle.Render("Jobs") + "\n"
	s += dimStyle.Render("ID                    Type       Status      Prio Age") + "\n"
	s += dimStyle.Render("─────────────────────────────────────────────────────") + "\n"

	if len(m.jobs) == 0 {
		s += dimStyle.Render("No jobs")
		return s
	}

	for i, job := range m.jobs {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
			s += selectedStyle.Render(fmt.Sprintf("%s %-18s %-9s %-10s %3d %s",
				prefix, job.ID[:8], job.Type, job.Status, job.Priority, job.Age))
		} else {
			statusColor := dimStyle
			if job.Status == "succeeded" {
				statusColor = successStyle
			} else if job.Status == "failed" || job.Status == "dead" {
				statusColor = errorStyle
			} else if job.Status == "running" {
				statusColor = infoStyle
			}
			s += fmt.Sprintf("%s %-18s %-9s ", prefix, job.ID[:8], job.Type)
			s += statusColor.Render(job.Status)
			s += fmt.Sprintf(" %3d %s\n", job.Priority, job.Age)
		}
	}
	return s
}

func (m *Model) workersView() string {
	s := infoStyle.Render("Workers") + "\n"
	s += dimStyle.Render("ID          Status   Active Job") + "\n"
	s += dimStyle.Render("─────────────────────────────────────") + "\n"

	if len(m.workers) == 0 {
		s += dimStyle.Render("No workers")
		return s
	}

	for _, w := range m.workers {
		statusColor := dimStyle
		if w.Status == "running" {
			statusColor = successStyle
		}
		s += fmt.Sprintf("%-10s %s %s\n", w.ID[:10], statusColor.Render(w.Status), dimStyle.Render(w.Active))
	}
	return s
}

func (m *Model) logsView() string {
	s := infoStyle.Render("Logs (Selected Job)") + "\n"
	s += dimStyle.Render("────────────────────────────────────────────────────────") + "\n"

	if len(m.logs) == 0 {
		s += dimStyle.Render("Select a job to view logs")
		return s
	}

	for _, log := range m.logs {
		s += dimStyle.Render(log) + "\n"
	}
	return s
}

func (m *Model) SetJobs(jobs []JobView) {
	m.jobs = jobs
}

func (m *Model) SetWorkers(workers []WorkerView) {
	m.workers = workers
}

func (m *Model) SetLogs(logs []string) {
	m.logs = logs
}

func (m *Model) GetSelectedJobID() string {
	if m.selected >= 0 && m.selected < len(m.jobs) {
		return m.jobs[m.selected].ID
	}
	return ""
}

type StateResponse struct {
	Jobs    []JobView    `json:"jobs"`
	Workers []WorkerView `json:"workers"`
}

func fetchDataCmd(serverURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(serverURL + "/state")
		if err != nil {
			return ErrorMsg(err.Error())
		}
		defer resp.Body.Close()

		var state StateResponse
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return ErrorMsg(err.Error())
		}
		return DataMsgFromState(state)
	}
}
