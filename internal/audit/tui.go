package audit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/model"
)

var (
	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")) // bright blue

	inactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")) // dim gray

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	activeHeaderStyle = headerStyle.
				Foreground(lipgloss.Color("39"))

	inactiveHeaderStyle = headerStyle.
				Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))

	jobTitleStyle = lipgloss.NewStyle().
			Bold(true)

	jobSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))
)

type auditModel struct {
	allJobs       []model.Job
	matchedJobs   []model.Job
	leftViewport  viewport.Model
	rightViewport viewport.Model
	activePane    int // 0=left, 1=right
	width         int
	height        int
	filterCfg     config.FilterConfig
	ready         bool
}

func (m auditModel) Init() tea.Cmd {
	return nil
}

func (m auditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "left", "right":
			m.activePane = 1 - m.activePane
			return m, nil
		}

		// Forward scroll keys to the active viewport.
		var cmd tea.Cmd
		if m.activePane == 0 {
			m.leftViewport, cmd = m.leftViewport.Update(msg)
		} else {
			m.rightViewport, cmd = m.rightViewport.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m *auditModel) recalcLayout() {
	// 2 border chars per pane + 1 gap between panes.
	paneWidth := max((m.width-5)/2, 20)

	// Header (1 line) + border top/bottom (2) + status bar (1) = 4 lines overhead.
	paneHeight := max(m.height-4, 5)

	if !m.ready {
		m.leftViewport = viewport.New(paneWidth, paneHeight)
		m.rightViewport = viewport.New(paneWidth, paneHeight)
		m.ready = true
	} else {
		m.leftViewport.Width = paneWidth
		m.leftViewport.Height = paneHeight
		m.rightViewport.Width = paneWidth
		m.rightViewport.Height = paneHeight
	}

	m.leftViewport.SetContent(renderJobs(m.allJobs))
	m.rightViewport.SetContent(renderJobs(m.matchedJobs))
}

func (m auditModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	paneWidth := m.leftViewport.Width

	// Headers.
	leftHeader := fmt.Sprintf(" All Jobs (%d)", len(m.allJobs))
	rightHeader := fmt.Sprintf(" Matched Jobs (%d)", len(m.matchedJobs))

	var leftHeaderRendered, rightHeaderRendered string
	var leftBorder, rightBorder lipgloss.Style

	if m.activePane == 0 {
		leftHeaderRendered = activeHeaderStyle.Render(leftHeader)
		rightHeaderRendered = inactiveHeaderStyle.Render(rightHeader)
		leftBorder = activeBorderStyle.Width(paneWidth)
		rightBorder = inactiveBorderStyle.Width(paneWidth)
	} else {
		leftHeaderRendered = inactiveHeaderStyle.Render(leftHeader)
		rightHeaderRendered = activeHeaderStyle.Render(rightHeader)
		leftBorder = inactiveBorderStyle.Width(paneWidth)
		rightBorder = activeBorderStyle.Width(paneWidth)
	}

	// Panes with borders.
	leftPane := leftBorder.Render(m.leftViewport.View())
	rightPane := rightBorder.Render(m.rightViewport.View())

	// Headers side by side.
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(paneWidth+2).Render(leftHeaderRendered),
		" ",
		lipgloss.NewStyle().Width(paneWidth+2).Render(rightHeaderRendered),
	)

	// Panes side by side.
	panes := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)

	// Status bar.
	filteredCount := len(m.allJobs) - len(m.matchedJobs)
	statusText := fmt.Sprintf(" %d total | %d matched | %d filtered out    ←/→/Tab switch pane  ↑/↓/j/k scroll  q quit",
		len(m.allJobs), len(m.matchedJobs), filteredCount)
	statusBar := statusBarStyle.Width(m.width).Render(statusText)

	return headerRow + "\n" + panes + "\n" + statusBar
}

func renderJobs(jobs []model.Job) string {
	if len(jobs) == 0 {
		return "  (no jobs)"
	}

	var b strings.Builder
	for i, j := range jobs {
		title := jobTitleStyle.Render(j.Title)
		b.WriteString(title)
		b.WriteByte('\n')

		posted := "n/a"
		if j.PostedAt != nil {
			posted = j.PostedAt.Format("2006-01-02")
		}
		subtitle := jobSubtitleStyle.Render(fmt.Sprintf("%s · %s", j.Location, posted))
		b.WriteString(subtitle)
		b.WriteByte('\n')

		if i < len(jobs)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sortJobsByDate(jobs []model.Job) {
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].PostedAt == nil && jobs[j].PostedAt == nil {
			return false
		}
		if jobs[i].PostedAt == nil {
			return false
		}
		if jobs[j].PostedAt == nil {
			return true
		}
		return jobs[i].PostedAt.After(*jobs[j].PostedAt)
	})
}

// RunAuditTUI launches the interactive split-pane audit TUI.
func RunAuditTUI(allJobs, matchedJobs []model.Job, filterCfg config.FilterConfig) error {
	sortJobsByDate(allJobs)
	sortJobsByDate(matchedJobs)

	m := auditModel{
		allJobs:     allJobs,
		matchedJobs: matchedJobs,
		filterCfg:   filterCfg,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
