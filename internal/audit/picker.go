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
)

type pickerModel struct {
	companies []config.CompanyConfig
	cursor    int
	chosen    int // -1 = no choice yet, -2 = quit
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	s := pickerBannerStyle.Render(pickerASCIIArt) + "\n"
	s += pickerSubtitleStyle.Render("job radar · be first in the door") + "\n"
	s += pickerTitleStyle.Render("Filter Audit — Select a company")
	s += "\n"

	for i, c := range m.companies {
		num := fmt.Sprintf("%2d.", i+1)
		ats := pickerATSStyle.Render(fmt.Sprintf("(%s)", c.ATS))
		label := fmt.Sprintf("%s %s %s", num, c.Name, ats)
		if i == m.cursor {
			s += pickerSelectedStyle.Render("> "+label) + "\n"
		} else {
			s += pickerItemStyle.Render(label) + "\n"
		}
	}

	s += pickerHintStyle.Render("↑/↓/j/k navigate  enter select  q quit")
	return s
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

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return -1, err
	}

	final := result.(pickerModel)
	return final.chosen, nil
}
