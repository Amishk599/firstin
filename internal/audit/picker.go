package audit

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amishk599/firstin/internal/config"
)

var (
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
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.companies)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	s := pickerTitleStyle.Render("Filter Audit — Select a company")
	s += "\n"

	for i, c := range m.companies {
		label := fmt.Sprintf("%s (%s)", c.Name, c.ATS)
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
