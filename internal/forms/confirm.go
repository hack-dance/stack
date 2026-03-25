package forms

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func Confirm(title string, description string) (bool, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false, fmt.Errorf("%s; rerun with --yes in a non-interactive shell", description)
	}

	value := false
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Continue").
				Negative("Cancel").
				Value(&value),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return value, nil
}
