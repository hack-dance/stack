package ui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

func RenderMarkdown(markdown string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return markdown
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}

	return strings.TrimSpace(rendered) + "\n"
}
