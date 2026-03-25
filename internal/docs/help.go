package docs

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func CommandMarkdown(cmd *cobra.Command) string {
	var sections []string

	sections = append(sections, "# "+cmd.CommandPath())
	sections = append(sections, cmd.Short)

	if strings.TrimSpace(cmd.Long) != "" {
		sections = append(sections, "## Details\n"+strings.TrimSpace(cmd.Long))
	}

	sections = append(sections, "## Usage\n```bash\n"+cmd.UseLine()+"\n```")

	if cmd.HasAvailableSubCommands() {
		lines := make([]string, 0)
		for _, sub := range cmd.Commands() {
			if !sub.IsAvailableCommand() || sub.Hidden {
				continue
			}
			lines = append(lines, fmt.Sprintf("- `%s` — %s", sub.Name(), sub.Short))
		}
		if len(lines) > 0 {
			sections = append(sections, "## Commands\n"+strings.Join(lines, "\n"))
		}
	}

	flags := make([]string, 0)
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		name := fmt.Sprintf("`--%s`", flag.Name)
		if flag.Shorthand != "" {
			name = fmt.Sprintf("`-%s`, %s", flag.Shorthand, name)
		}
		flags = append(flags, fmt.Sprintf("- %s — %s", name, flag.Usage))
	})
	if len(flags) > 0 {
		sections = append(sections, "## Flags\n"+strings.Join(flags, "\n"))
	}

	if strings.TrimSpace(cmd.Example) != "" {
		sections = append(sections, "## Examples\n```bash\n"+strings.TrimSpace(cmd.Example)+"\n```")
	}

	return strings.Join(sections, "\n\n")
}
