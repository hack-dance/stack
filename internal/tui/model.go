package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hack-dance/stack/internal/stack"
	"github.com/hack-dance/stack/internal/ui"
)

type item struct {
	title       string
	description string
	value       string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.value }

type keyMap struct {
	Up   key.Binding
	Down key.Binding
	Help key.Binding
	Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Help, k.Quit}}
}

type model struct {
	list     list.Model
	viewport viewport.Model
	help     help.Model
	keys     keyMap
	summary  stack.Summary
	width    int
	height   int
}

func Run(summary stack.Summary) error {
	items := make([]list.Item, 0, len(summary.Branches))
	for _, branch := range summary.Branches {
		label := branch.Parent
		if len(branch.Issues) > 0 {
			label = branch.Issues[0].Message
		}
		items = append(items, item{
			title:       branch.Name,
			description: label,
			value:       branch.Name,
		})
	}

	branchList := list.New(items, list.NewDefaultDelegate(), 0, 0)
	branchList.SetFilteringEnabled(false)
	branchList.SetShowHelp(false)
	branchList.SetShowStatusBar(false)
	branchList.Title = "Tracked branches"

	keys := keyMap{
		Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move")),
		Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move")),
		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle help")),
		Quit: key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
	}

	m := model{
		list:     branchList,
		viewport: viewport.New(0, 0),
		help:     help.New(),
		keys:     keys,
		summary:  summary,
	}
	m.syncViewport()

	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		listWidth := max(32, typed.Width/3)
		m.list.SetSize(listWidth, typed.Height-6)
		m.viewport.Width = max(40, typed.Width-listWidth-8)
		m.viewport.Height = max(10, typed.Height-8)
		m.syncViewport()
		return m, nil
	case tea.KeyMsg:
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		}
	}

	nextList, cmd := m.list.Update(msg)
	m.list = nextList
	m.syncViewport()
	return m, cmd
}

func (m model) View() string {
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		ui.TitleStyle.Render("stack tui"),
		ui.SubtitleStyle.Render(fmt.Sprintf("%s  •  trunk %s", m.summary.RepoRoot, m.summary.Trunk)),
	)

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ui.AppFrameStyle.Width(m.list.Width()+4).Render(m.list.View()),
		"  ",
		ui.AppFrameStyle.Width(m.viewport.Width+4).Render(m.viewport.View()),
	)

	footer := ui.MutedStyle.Render(strings.TrimSpace(m.help.View(m.keys)))
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
}

func (m *model) syncViewport() {
	if len(m.summary.Branches) == 0 {
		m.viewport.SetContent("No tracked branches yet.")
		return
	}

	selected, ok := m.list.SelectedItem().(item)
	if !ok {
		m.viewport.SetContent("No branch selected.")
		return
	}

	for _, branch := range m.summary.Branches {
		if branch.Name != selected.value {
			continue
		}

		lines := []string{
			ui.TitleStyle.Render(branch.Name),
			ui.MutedStyle.Render(fmt.Sprintf("parent: %s", branch.Parent)),
			ui.MutedStyle.Render(fmt.Sprintf("local exists: %t", branch.LocalExists)),
			ui.MutedStyle.Render(fmt.Sprintf("remote exists: %t", branch.RemoteExists)),
			ui.MutedStyle.Render(fmt.Sprintf("parent ancestor: %t", branch.ParentAncestor)),
			"",
			ui.SectionStyle.Render("Health"),
		}

		if len(m.summary.RepoIssues) > 0 {
			lines = append(lines, "")
			lines = append(lines, ui.SectionStyle.Render("Repository"))
			for _, issue := range m.summary.RepoIssues {
				lines = append(lines, fmt.Sprintf("- %s", issue.Message))
			}
			lines = append(lines, "")
			lines = append(lines, ui.SectionStyle.Render("Branch"))
		}

		if len(branch.Issues) == 0 {
			lines = append(lines, ui.HealthyBadgeStyle.Render("No issues detected."))
		} else {
			for _, issue := range branch.Issues {
				lines = append(lines, fmt.Sprintf("- %s", issue.Message))
			}
		}

		if branch.Record.PR.Number > 0 {
			lines = append(
				lines,
				"",
				ui.SectionStyle.Render("Pull request"),
				ui.MutedStyle.Render(fmt.Sprintf("#%d  %s", branch.Record.PR.Number, branch.Record.PR.URL)),
				ui.MutedStyle.Render(fmt.Sprintf("state: %s  draft: %t", branch.Record.PR.State, branch.Record.PR.IsDraft)),
			)
		}

		m.viewport.SetContent(strings.Join(lines, "\n"))
		return
	}

	m.viewport.SetContent("Branch not found.")
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
