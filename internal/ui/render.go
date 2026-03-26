package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hack-dance/stack/internal/stack"
)

func RenderStatus(summary stack.Summary) string {
	lines := []string{
		TitleStyle.Render("stack status"),
		SubtitleStyle.Render(fmt.Sprintf("%s  •  trunk %s  •  remote %s", summary.RepoRoot, summary.Trunk, summary.DefaultRemote)),
		"",
		SectionStyle.Render("Repository"),
	}

	if len(summary.RepoIssues) == 0 {
		lines = append(lines, MutedStyle.Render("No repository-level issues detected."))
	} else {
		for _, issue := range summary.RepoIssues {
			lines = append(lines, fmt.Sprintf("- %s", renderIssue(issue)))
		}
	}

	lines = append(lines,
		"",
		SectionStyle.Render("Branches"),
	)

	if len(summary.Branches) == 0 {
		lines = append(lines, MutedStyle.Render("No tracked branches yet. Start with `stack init` then `stack create <branch>`."))
		return AppFrameStyle.Render(strings.Join(lines, "\n"))
	}

	for _, branch := range summary.Branches {
		prefix := strings.Repeat("  ", branch.Depth)
		status := HealthyBadgeStyle.Render("healthy")
		if len(branch.Issues) > 0 {
			status = renderSeverity(branch.Issues[0].Severity)
		}

		currentMarker := ""
		if branch.IsCurrentBranch {
			currentMarker = " " + CodeStyle.Render("(current)")
		}

		lines = append(
			lines,
			fmt.Sprintf("%s%s %s%s", prefix, status, CodeStyle.Render(branch.Name), currentMarker),
			MutedStyle.Render(fmt.Sprintf("%sparent: %s", prefix+"  ", branch.Parent)),
		)

		if branch.Record.PR.Number > 0 {
			lines = append(
				lines,
				MutedStyle.Render(
					fmt.Sprintf("%sPR: #%d %s", prefix+"  ", branch.Record.PR.Number, branch.Record.PR.State),
				),
			)
		}
		if branch.Verification != nil {
			lines = append(lines, MutedStyle.Render(fmt.Sprintf("%sVerify: %s", prefix+"  ", renderVerificationSummary(*branch.Verification))))
		}

		if len(branch.Issues) == 0 {
			lines = append(lines, MutedStyle.Render(fmt.Sprintf("%sNo issues detected.", prefix+"  ")))
			continue
		}

		for _, issue := range branch.Issues {
			lines = append(lines, fmt.Sprintf("%s- %s", prefix+"  ", renderIssue(issue)))
		}
	}

	if len(summary.LandingBranches) > 0 {
		lines = append(lines,
			"",
			SectionStyle.Render("Landing Branches"),
		)
		for _, branch := range summary.LandingBranches {
			status := HealthyBadgeStyle.Render("healthy")
			if len(branch.Issues) > 0 {
				status = renderSeverity(branch.Issues[0].Severity)
			}

			currentMarker := ""
			if branch.IsCurrentBranch {
				currentMarker = " " + CodeStyle.Render("(current)")
			}

			lines = append(lines, fmt.Sprintf("%s %s%s", status, CodeStyle.Render(branch.Name), currentMarker))
			if branch.Verification != nil {
				lines = append(lines, MutedStyle.Render(fmt.Sprintf("  Verify: %s", renderVerificationSummary(*branch.Verification))))
			}
			if len(branch.Issues) == 0 {
				lines = append(lines, MutedStyle.Render("  No issues detected."))
				continue
			}
			for _, issue := range branch.Issues {
				lines = append(lines, fmt.Sprintf("  - %s", renderIssue(issue)))
			}
		}
	}

	return AppFrameStyle.Render(strings.Join(lines, "\n"))
}

func RenderPreview(title string, lines []string) string {
	body := append([]string{TitleStyle.Render(title), ""}, lines...)
	return AppFrameStyle.Render(strings.Join(body, "\n"))
}

func renderSeverity(severity stack.Severity) string {
	switch severity {
	case stack.SeverityError:
		return ErrorBadgeStyle.Render("error")
	case stack.SeverityWarn:
		return WarnBadgeStyle.Render("warn")
	case stack.SeverityInfo:
		return InfoBadgeStyle.Render("info")
	default:
		return HealthyBadgeStyle.Render("healthy")
	}
}

func renderIssue(issue stack.HealthIssue) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		renderSeverity(issue.Severity),
		" ",
		issue.Message,
	)
}

func renderVerificationSummary(summary stack.VerificationSummary) string {
	result := verificationResultStyle(summary.Latest.Passed).Render(verificationResultLabel(summary.Latest.Passed))
	line := fmt.Sprintf("%s %s", summary.Latest.CheckType, result)
	if summary.Latest.Identifier != "" {
		line += fmt.Sprintf(" %s", summary.Latest.Identifier)
	}
	if summary.Latest.Score != nil {
		line += fmt.Sprintf(" score=%d", *summary.Latest.Score)
	}
	if !summary.HeadMatchesCurrent {
		line += " stale"
	}
	line += fmt.Sprintf(" (%d total)", summary.Count)
	return line
}

func verificationResultStyle(passed bool) lipgloss.Style {
	if passed {
		return HealthyBadgeStyle
	}
	return WarnBadgeStyle
}

func verificationResultLabel(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}
