package audit

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amishk599/firstin/internal/config"
)

const pickerASCIIArt = `███████╗██╗██████╗ ███████╗████████╗██╗███╗   ██╗
██╔════╝██║██╔══██╗██╔════╝╚══██╔══╝██║████╗  ██║
█████╗  ██║██████╔╝███████╗   ██║   ██║██╔██╗ ██║
██╔══╝  ██║██╔══██╗╚════██║   ██║   ██║██║╚██╗██║
██║     ██║██║  ██║███████║   ██║   ██║██║ ╚████║
╚═╝     ╚═╝╚═╝  ╚═╝╚══════╝   ╚═╝   ╚═╝╚═╝  ╚═══╝`

var (
	pickerBannerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Padding(1, 0, 0, 2)

	pickerSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Padding(0, 0, 0, 2)

	pickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Padding(1, 0, 1, 2)

	pickerItemStyle = lipgloss.NewStyle().
			Padding(0, 0, 0, 4)

	pickerSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true).
				Padding(0, 0, 0, 2)

	pickerHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 0, 0, 2)

	pickerATSStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	pickerDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238")).
				Padding(0, 0, 0, 4)

	pickerDisabledSelectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("238")).
					Bold(true).
					Padding(0, 0, 0, 2)
)

// fixedPickerLines is the number of lines consumed by the banner, subtitle,
// title, and hint — everything except the scrollable company list.
// banner: 1 top-pad + 6 ASCII lines + 1 trailing \n = 8
// subtitle: 1 line + 1 trailing \n                   = 2
// title: 1 top-pad + 1 line + 1 bottom-pad + 1 \n   = 4
// hint: 1 top-pad + 1 line                           = 2
const fixedPickerLines = 16

type pickerModel struct {
	companies []config.CompanyConfig
	cursor    int
	chosen    int // -1 = no choice yet, -2 = quit
	height    int
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.chosen = -2
			return m, tea.Quit
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(m.companies)) % len(m.companies)
		case "down", "j":
			m.cursor = (m.cursor + 1) % len(m.companies)
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	header := pickerBannerStyle.Render(pickerASCIIArt) + "\n"
	header += pickerSubtitleStyle.Render("job radar · be first in the door") + "\n"
	header += pickerTitleStyle.Render("Filter Audit — Select a company") + "\n"
	hint := pickerHintStyle.Render("↑/↓/j/k navigate  enter select  q quit")

	// Determine how many list rows fit between the header and hint.
	available := len(m.companies)
	if m.height > 0 {
		available = m.height - fixedPickerLines
		if available < 1 {
			available = 1
		}
	}

	// Compute scroll offset so the cursor row is always visible.
	scrollOffset := 0
	if m.cursor >= available {
		scrollOffset = m.cursor - available + 1
	}

	end := scrollOffset + available
	if end > len(m.companies) {
		end = len(m.companies)
	}

	var list strings.Builder
	for i := scrollOffset; i < end; i++ {
		c := m.companies[i]
		num := fmt.Sprintf("%2d.", i+1)
		ats := pickerATSStyle.Render(fmt.Sprintf("(%s)", c.ATS))
		displayName := strings.ToUpper(c.Name[:1]) + c.Name[1:]
		if !c.Enabled {
			displayName += " [disabled]"
		}
		label := fmt.Sprintf("%s %s %s", num, displayName, ats)
		if i == m.cursor {
			if c.Enabled {
				list.WriteString(pickerSelectedStyle.Render("> "+label) + "\n")
			} else {
				list.WriteString(pickerDisabledSelectedStyle.Render("> "+label) + "\n")
			}
		} else {
			if c.Enabled {
				list.WriteString(pickerItemStyle.Render(label) + "\n")
			} else {
				list.WriteString(pickerDisabledStyle.Render(label) + "\n")
			}
		}
	}

	return header + list.String() + hint
}

// RunCompanyPicker shows an interactive company selector.
// Returns the index of the chosen company, or -1 if the user quit.
func RunCompanyPicker(companies []config.CompanyConfig) (int, error) {
	sort.Slice(companies, func(i, j int) bool {
		return strings.ToLower(companies[i].Name) < strings.ToLower(companies[j].Name)
	})

	m := pickerModel{
		companies: companies,
		chosen:    -1,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return -1, err
	}

	final := result.(pickerModel)
	return final.chosen, nil
}
