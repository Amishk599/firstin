package audit

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/poller"
)

var pst = time.FixedZone("PST", -8*60*60)

func fmtTimePST(t *time.Time, layout string) string {
	return t.In(pst).Format(layout)
}

// Lines per job item in the list view (title + subtitle + blank separator).
const jobItemHeight = 3

type viewState int

const (
	viewList viewState = iota
	viewDetail
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

	selectedJobTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")). // bright white
				Background(lipgloss.Color("24"))  // dark blue bg

	selectedJobSubtitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("252")).
					Background(lipgloss.Color("24"))

	detailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Width(16)

	detailValueStyle = lipgloss.NewStyle()

	detailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				MarginBottom(1)

	descDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	descHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Italic(true)

	descBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
)

// detailFetchedMsg is sent when an async detail fetch completes.
type detailFetchedMsg struct {
	job model.Job
	err error
}

// jobAnalyzedMsg is sent when an async AI analysis completes.
type jobAnalyzedMsg struct {
	job model.Job
	err error
}

type auditModel struct {
	allJobs       []model.Job
	matchedJobs   []model.Job
	leftViewport  viewport.Model
	rightViewport viewport.Model
	activePane    int // 0=left, 1=right
	leftCursor    int
	rightCursor   int
	width         int
	height        int
	filterCfg     config.FilterConfig
	ready         bool

	// Detail view state
	view            viewState
	detailJob       model.Job
	detailLoading   bool
	detailError     string
	detailViewport  viewport.Model
	detailFetcher   model.JobDetailFetcher
	showDescription bool

	// AI analysis state
	analyzer      poller.JobAnalyzer
	analyzeLoading bool
	analyzeError   string

	wantQuit bool
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
		if m.view == viewDetail {
			m.detailViewport.Width = m.width - 4
			m.detailViewport.Height = m.height - 4
			m.detailViewport.SetContent(m.renderDetail())
		}
		return m, nil

	case detailFetchedMsg:
		m.detailLoading = false
		if msg.err != nil {
			m.detailError = fmt.Sprintf("failed to load description: %v", msg.err)
			m.detailViewport.SetContent(m.renderDetail())
			return m, nil
		}
		m.detailError = ""
		m.detailJob = msg.job
		// Update the job in the list so re-entering doesn't re-fetch
		m.updateJobInLists(msg.job)
		m.detailViewport.SetContent(m.renderDetail())
		return m, nil

	case jobAnalyzedMsg:
		m.analyzeLoading = false
		if msg.err != nil {
			m.analyzeError = fmt.Sprintf("analysis failed: %v", msg.err)
		} else if msg.job.Insights == nil {
			m.analyzeError = "AI enrichment is not enabled — set ai.enabled: true in config.yaml"
		} else {
			m.analyzeError = ""
			m.detailJob = msg.job
			m.updateJobInLists(msg.job)
		}
		m.detailViewport.SetContent(m.renderDetail())
		return m, nil

	case tea.KeyMsg:
		if m.view == viewDetail {
			return m.updateDetailView(msg)
		}
		return m.updateListView(msg)
	}

	return m, nil
}

func (m auditModel) updateListView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.wantQuit = true
		return m, tea.Quit
	case "esc", "b":
		m.wantQuit = false
		return m, tea.Quit
	case "tab", "left", "right":
		m.activePane = 1 - m.activePane
		m.recalcContent()
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		m.recalcContent()
		m.ensureCursorVisible()
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		m.recalcContent()
		m.ensureCursorVisible()
		return m, nil
	case "enter":
		return m.openDetailView()
	}

	// Forward other keys (pgup/pgdn/home/end) to the active viewport.
	var cmd tea.Cmd
	if m.activePane == 0 {
		m.leftViewport, cmd = m.leftViewport.Update(msg)
	} else {
		m.rightViewport, cmd = m.rightViewport.Update(msg)
	}
	return m, cmd
}

func (m auditModel) updateDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.wantQuit = true
		return m, tea.Quit
	case "esc", "backspace":
		m.view = viewList
		return m, nil
	case "o":
		url := m.detailJob.URL
		if m.detailJob.Detail != nil && m.detailJob.Detail.ApplyURL != "" {
			url = m.detailJob.Detail.ApplyURL
		}
		openURL(url)
		return m, nil
	case "r":
		if m.detailJob.Detail != nil && m.detailJob.Detail.Description != "" {
			m.showDescription = !m.showDescription
			m.detailViewport.SetContent(m.renderDetail())
			m.detailViewport.SetYOffset(0)
		}
		return m, nil
	case "s":
		if m.analyzer != nil && !m.analyzeLoading && m.detailJob.Insights == nil &&
			m.detailJob.Detail != nil && m.detailJob.Detail.Description != "" {
			m.analyzeLoading = true
			m.analyzeError = ""
			m.detailViewport.SetContent(m.renderDetail())
			return m, m.analyzeJobCmd(m.detailJob)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.detailViewport, cmd = m.detailViewport.Update(msg)
	return m, cmd
}

func (m auditModel) analyzeJobCmd(job model.Job) tea.Cmd {
	analyzer := m.analyzer
	return func() tea.Msg {
		analyzed, err := analyzer.Analyze(context.Background(), job)
		return jobAnalyzedMsg{job: analyzed, err: err}
	}
}

func (m *auditModel) moveCursor(delta int) {
	if m.activePane == 0 {
		m.leftCursor = clamp(m.leftCursor+delta, 0, max(len(m.allJobs)-1, 0))
	} else {
		m.rightCursor = clamp(m.rightCursor+delta, 0, max(len(m.matchedJobs)-1, 0))
	}
}

func (m *auditModel) ensureCursorVisible() {
	var vp *viewport.Model
	var cursor int
	if m.activePane == 0 {
		vp = &m.leftViewport
		cursor = m.leftCursor
	} else {
		vp = &m.rightViewport
		cursor = m.rightCursor
	}

	cursorTop := cursor * jobItemHeight
	cursorBottom := cursorTop + jobItemHeight - 1

	if cursorTop < vp.YOffset {
		vp.SetYOffset(cursorTop)
	} else if cursorBottom >= vp.YOffset+vp.Height {
		vp.SetYOffset(cursorBottom - vp.Height + 1)
	}
}

func (m auditModel) openDetailView() (tea.Model, tea.Cmd) {
	jobs := m.activeJobs()
	cursor := m.activeCursor()
	if len(jobs) == 0 {
		return m, nil
	}

	job := jobs[cursor]
	m.view = viewDetail
	m.detailJob = job
	m.detailError = ""
	m.showDescription = false
	m.detailViewport = viewport.New(m.width-4, m.height-4)
	m.detailViewport.SetContent(m.renderDetail())

	// If we have a detail fetcher and the job lacks enriched detail, fetch it
	if m.detailFetcher != nil && !hasEnrichedDetail(job) {
		m.detailLoading = true
		return m, m.fetchDetailCmd(job)
	}

	return m, nil
}

func (m auditModel) fetchDetailCmd(job model.Job) tea.Cmd {
	fetcher := m.detailFetcher
	return func() tea.Msg {
		enriched, err := fetcher.FetchJobDetail(context.Background(), job)
		return detailFetchedMsg{job: enriched, err: err}
	}
}

func (m *auditModel) updateJobInLists(job model.Job) {
	for i := range m.allJobs {
		if m.allJobs[i].ID == job.ID {
			m.allJobs[i] = job
			break
		}
	}
	for i := range m.matchedJobs {
		if m.matchedJobs[i].ID == job.ID {
			m.matchedJobs[i] = job
			break
		}
	}
}

func hasEnrichedDetail(job model.Job) bool {
	if job.Detail == nil {
		return false
	}
	d := job.Detail
	return d.RequisitionID != "" || len(d.PayRanges) > 0 || d.ApplyURL != "" || d.Description != ""
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

	m.recalcContent()
}

func (m *auditModel) recalcContent() {
	m.leftViewport.SetContent(renderJobs(m.allJobs, m.leftCursor, m.activePane == 0))
	m.rightViewport.SetContent(renderJobs(m.matchedJobs, m.rightCursor, m.activePane == 1))
}

func (m auditModel) activeJobs() []model.Job {
	if m.activePane == 0 {
		return m.allJobs
	}
	return m.matchedJobs
}

func (m auditModel) activeCursor() int {
	if m.activePane == 0 {
		return m.leftCursor
	}
	return m.rightCursor
}

func (m auditModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.view == viewDetail {
		return m.viewDetail()
	}

	return m.viewList()
}

func (m auditModel) viewList() string {
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
	statusText := fmt.Sprintf(" %d total | %d matched | %d filtered out    ←/→/Tab switch  ↑/↓ cursor  Enter detail  Esc back  q quit",
		len(m.allJobs), len(m.matchedJobs), filteredCount)
	statusBar := statusBarStyle.Width(m.width).Render(statusText)

	return headerRow + "\n" + panes + "\n" + statusBar
}

func (m auditModel) viewDetail() string {
	title := detailTitleStyle.Render("Job Details")
	if m.detailLoading {
		title += "  (loading...)"
	}

	border := activeBorderStyle.Width(m.width - 2)
	content := border.Render(m.detailViewport.View())

	statusText := " o open URL  esc/backspace back  ↑/↓ scroll  q quit"
	if m.detailJob.Detail != nil && m.detailJob.Detail.Description != "" {
		if m.analyzer != nil && m.detailJob.Insights == nil && !m.analyzeLoading {
			statusText = " o open URL  r desc  s summary  esc/backspace back  ↑/↓ scroll  q quit"
		} else {
			statusText = " o open URL  r desc  esc/backspace back  ↑/↓ scroll  q quit"
		}
	}
	statusBar := statusBarStyle.Width(m.width).Render(statusText)

	return title + "\n" + content + "\n" + statusBar
}

func (m auditModel) renderDetail() string {
	j := m.detailJob
	var b strings.Builder

	addField := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(detailLabelStyle.Render(label))
		b.WriteString(detailValueStyle.Render(value))
		b.WriteByte('\n')
	}

	addField("Title", j.Title)
	addField("Company", j.Company)
	addField("Location", j.Location)
	addField("Job ID", j.ID)
	addField("Source", j.Source)

	b.WriteByte('\n')

	if j.PostedAt != nil {
		addField("Posted At", fmtTimePST(j.PostedAt, "2006-01-02 15:04 MST"))
	}

	if j.Detail != nil {
		d := j.Detail

		if d.UpdatedAt != nil {
			addField("Updated At", fmtTimePST(d.UpdatedAt, "2006-01-02 15:04 MST"))
		}
		if d.FirstPublished != nil {
			addField("First Published", fmtTimePST(d.FirstPublished, "2006-01-02 15:04 MST"))
		}
		if d.StartDate != nil {
			addField("Start Date", fmtTimePST(d.StartDate, "2006-01-02 MST"))
		}
		if d.PublishedAt != nil {
			addField("Published At", fmtTimePST(d.PublishedAt, "2006-01-02 15:04 MST"))
		}
		if d.PostedOn != "" {
			addField("Posted On", d.PostedOn)
		}
		if d.RequisitionID != "" {
			addField("Requisition ID", d.RequisitionID)
		}

		if len(d.PayRanges) > 0 {
			b.WriteByte('\n')
			for _, pr := range d.PayRanges {
				rangeStr := formatPayRange(pr)
				label := "Pay Range"
				if pr.Title != "" {
					label = pr.Title
				}
				addField(label, rangeStr)
			}
		}
	}

	b.WriteByte('\n')
	addField("Job URL", j.URL)
	if j.Detail != nil && j.Detail.ApplyURL != "" && j.Detail.ApplyURL != j.URL {
		addField("Apply URL", j.Detail.ApplyURL)
	}

	if m.detailError != "" {
		b.WriteByte('\n')
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("⚠ "+m.detailError) + "\n")
	}

	// AI insights block
	wrapWidth := max(m.width-8, 20)
	divider := func(label string) string {
		fill := strings.Repeat("─", max(wrapWidth-len(label), 3))
		return descDividerStyle.Render(label+fill)
	}
	if j.Insights != nil {
		ins := j.Insights
		b.WriteByte('\n')
		b.WriteString(divider("── AI Summary ") + "\n\n")
		addField("Role", ins.RoleType)
		addField("Experience", ins.YearsExp)
		if len(ins.TechStack) > 0 {
			addField("Stack", strings.Join(ins.TechStack, ", "))
		}
		b.WriteByte('\n')
		for _, pt := range ins.KeyPoints {
			if pt != "" {
				b.WriteString(detailValueStyle.Render("  • "+pt) + "\n")
			}
		}
	} else if m.analyzeLoading {
		b.WriteByte('\n')
		b.WriteString(descHintStyle.Render("  analyzing job description...") + "\n")
	} else if m.analyzeError == "" && j.Detail != nil && j.Detail.Description != "" {
		b.WriteByte('\n')
		b.WriteString(descHintStyle.Render("  press s for job description summary") + "\n")
	}

	if j.Detail != nil && j.Detail.Description != "" {
		b.WriteByte('\n')
		if m.showDescription {
			b.WriteString(divider("── Job Description ") + "\n\n")
			b.WriteString(descBodyStyle.Render(wordWrap(j.Detail.Description, wrapWidth)) + "\n")
		} else {
			hint := "  press r to read job description"
			b.WriteString(descHintStyle.Render(hint) + "\n")
		}
	}

	return b.String()
}

func formatPayRange(pr model.PayRange) string {
	minDollars := float64(pr.MinCents) / 100
	maxDollars := float64(pr.MaxCents) / 100
	currency := pr.CurrencyType
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%s $%.0f - $%.0f", currency, minDollars, maxDollars)
}

func renderJobs(jobs []model.Job, cursor int, isActive bool) string {
	if len(jobs) == 0 {
		return "  (no jobs)"
	}

	var b strings.Builder
	for i, j := range jobs {
		isSelected := isActive && i == cursor

		titleSt := jobTitleStyle
		subtitleSt := jobSubtitleStyle
		prefix := "  "
		if isSelected {
			titleSt = selectedJobTitleStyle
			subtitleSt = selectedJobSubtitleStyle
			prefix = "> "
		}

		b.WriteString(prefix)
		b.WriteString(titleSt.Render(j.Title))
		b.WriteByte('\n')

		posted := "n/a"
		if j.PostedAt != nil {
			posted = j.PostedAt.Format("2006-01-02")
		}
		b.WriteString(prefix)
		b.WriteString(subtitleSt.Render(fmt.Sprintf("%s · %s", j.Location, posted)))
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

func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// openURL opens url in the default system browser, fire-and-forget.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}

// RunAuditTUI launches the interactive split-pane audit TUI.
// detailFetcher may be nil for adapters that don't support on-demand detail fetching.
// analyzer may be nil; when non-nil the 's' key triggers AI analysis in the detail view.
// Returns wantQuit=true if the user pressed q/ctrl+c, false if they pressed esc to return to the picker.
func RunAuditTUI(allJobs, matchedJobs []model.Job, filterCfg config.FilterConfig, detailFetcher model.JobDetailFetcher, analyzer poller.JobAnalyzer) (bool, error) {
	sortJobsByDate(allJobs)
	sortJobsByDate(matchedJobs)

	m := auditModel{
		allJobs:       allJobs,
		matchedJobs:   matchedJobs,
		filterCfg:     filterCfg,
		detailFetcher: detailFetcher,
		analyzer:      analyzer,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return false, err
	}
	final := result.(auditModel)
	return final.wantQuit, nil
}
