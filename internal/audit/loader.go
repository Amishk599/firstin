package audit

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amishk599/firstin/internal/model"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type fetchDoneMsg struct {
	jobs []model.Job
	err  error
}

type spinnerTickMsg struct{}

type loaderModel struct {
	companyName string
	fetchFn     func(ctx context.Context) ([]model.Job, error)
	frame       int
	result      []model.Job
	err         error
	done        bool
}

func (m loaderModel) Init() tea.Cmd {
	return tea.Batch(m.doFetch(), m.tick())
}

func (m loaderModel) doFetch() tea.Cmd {
	fetchFn := m.fetchFn
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		jobs, err := fetchFn(ctx)
		return fetchDoneMsg{jobs: jobs, err: err}
	}
}

func (m loaderModel) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m loaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fetchDoneMsg:
		m.result = msg.jobs
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	case spinnerTickMsg:
		m.frame = (m.frame + 1) % len(spinnerFrames)
		return m, m.tick()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			m.err = fmt.Errorf("cancelled")
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m loaderModel) View() string {
	if m.done {
		return ""
	}
	spinner := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(spinnerFrames[m.frame])
	return fmt.Sprintf("%s Fetching jobs from %s...\n", spinner, m.companyName)
}

// RunLoader shows a spinner while fetching jobs. It renders inline (no alt screen).
func RunLoader(companyName string, fetchFn func(ctx context.Context) ([]model.Job, error)) ([]model.Job, error) {
	m := loaderModel{
		companyName: companyName,
		fetchFn:     fetchFn,
	}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return nil, err
	}
	final := result.(loaderModel)
	return final.result, final.err
}
