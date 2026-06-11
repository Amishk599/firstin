package audit

import (
	"errors"
	"fmt"

	"github.com/amishk599/firstin/internal/config"
	"github.com/charmbracelet/huh"
)

// RunNewCompanyForm runs an interactive form to collect details for a new company.
// Returns huh.ErrUserAborted if the user cancels.
func RunNewCompanyForm() (config.CompanyConfig, error) {
	var (
		name       string
		ats        string
		boardToken string
		workdayURL string
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Company name").
				Placeholder("e.g. acme").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}).
				Value(&name),

			huh.NewSelect[string]().
				Title("ATS").
				Options(
					huh.NewOption("Greenhouse", "greenhouse"),
					huh.NewOption("Ashby", "ashby"),
					huh.NewOption("Lever", "lever"),
					huh.NewOption("Gem", "gem"),
					huh.NewOption("Workday", "workday"),
					huh.NewOption("Microsoft (no token needed)", "microsoft"),
				).
				Value(&ats),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Board token").
				Placeholder("e.g. acme-inc").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}).
				Value(&boardToken),
		).WithHideFunc(func() bool {
			return ats == "workday" || ats == "microsoft"
		}),

		huh.NewGroup(
			huh.NewInput().
				Title("Workday base URL").
				Placeholder("e.g. https://acme.wd1.myworkdayjobs.com/careers").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}).
				Value(&workdayURL),
		).WithHideFunc(func() bool {
			return ats != "workday"
		}),
	)

	if err := form.Run(); err != nil {
		return config.CompanyConfig{}, fmt.Errorf("form: %w", err)
	}

	return config.CompanyConfig{
		Name:       name,
		ATS:        ats,
		BoardToken: boardToken,
		WorkdayURL: workdayURL,
		Enabled:    true,
	}, nil
}
