// Token panel renders real-time session stats in the right sidebar:
// remaining percentage bar, cache hit display, reset estimate, compaction warnings,
// and optional per-category token breakdown (toggled with 'b').
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/avaleror/claudewrap/internal/compact"
	"github.com/avaleror/claudewrap/internal/monitor"
)

func renderTokenPanel(snap monitor.StateSnapshot, width int, showBreakdown bool) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Claude Session") + "  " + dimStyle.Render("[b: breakdown]") + "\n")

	// Token bar
	remaining := snap.RemainingPct
	barWidth := width - 6
	if barWidth < 4 {
		barWidth = 4
	}
	barColor := colorGood
	if remaining <= 11 {
		barColor = colorDanger
	} else if remaining <= 30 {
		barColor = colorWarn
	}
	bar := monitor.ProgressBar(remaining, barWidth)
	b.WriteString(fmt.Sprintf("  Used:  %s tokens\n", formatTokens(snap.UsedTokens)))
	b.WriteString(fmt.Sprintf("  Rem:   %s\n",
		lipgloss.NewStyle().Foreground(barColor).Render(bar)))

	if snap.LastUsage != nil {
		cacheRead := snap.LastUsage.CacheReadInputTokens
		cacheCreate := snap.LastUsage.CacheCreationInputTokens
		if cacheRead > 0 || cacheCreate > 0 {
			saved := lipgloss.NewStyle().Foreground(colorGood).Render(fmt.Sprintf("↓%s", formatTokens(cacheRead)))
			b.WriteString(fmt.Sprintf("  Cache: %s read / %s new\n", saved, formatTokens(cacheCreate)))
		}
	}

	resetLine := "  Reset: " + snap.ResetIn()
	if snap.IsPeak {
		resetLine += "  " + lipgloss.NewStyle().Foreground(colorWarn).Render("Peak active")
	}
	b.WriteString(resetLine + "\n")

	// Compaction
	compCount := snap.CompactionCount
	compLabel := fmt.Sprintf("  Compacted: %dx", compCount)
	switch compact.GetWarning(compCount) {
	case compact.WarnQuality:
		b.WriteString(lipgloss.NewStyle().Foreground(colorWarn).Render(compLabel) + "\n")
	case compact.WarnRestart:
		b.WriteString(lipgloss.NewStyle().Foreground(colorDanger).Render(compLabel) + "\n")
	default:
		b.WriteString(dimStyle.Render(compLabel) + "\n")
	}

	warn := compact.WarningText(compCount)
	if warn != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colorWarn).Render("  "+warn) + "\n")
	}

	// Fallback cost
	if snap.FallbackEngine != "" {
		b.WriteString("\n" + headerStyle.Render("AI Fallback") + "\n")
		b.WriteString(fmt.Sprintf("  Today:  %d tokens / $%.4f\n",
			snap.FallbackDailyTokens, snap.FallbackDailyCost))
		b.WriteString(fmt.Sprintf("  Engine: %s\n", snap.FallbackEngine))
	}

	if showBreakdown && snap.LastUsage != nil && snap.LastUsage.Breakdown != nil {
		b.WriteString(renderBreakdown(snap.LastUsage.Breakdown, width))
	}

	return b.String()
}

func renderBreakdown(bd *monitor.Breakdown, width int) string {
	var b strings.Builder
	b.WriteString("\n" + headerStyle.Render("Token Breakdown") + "\n")

	rows := []struct {
		label string
		val   int
	}{
		{"CLAUDE.md", bd.ClaudeMD},
		{"Tool call I/O", bd.ToolCallIO},
		{"@-files", bd.MentionedFiles},
		{"Thinking", bd.ExtendedThinking},
		{"Conversation", bd.Conversation},
		{"Skills", bd.SkillActivations},
		{"Team", bd.TeamOverhead},
		{"User text", bd.UserText},
	}

	maxLabel := 12
	sep := strings.Repeat("─", width-2)
	for _, row := range rows {
		label := row.label
		if len(label) < maxLabel {
			label = label + strings.Repeat(" ", maxLabel-len(label))
		}
		b.WriteString(fmt.Sprintf("  %s %d\n", label, row.val))
	}
	b.WriteString("  " + sep + "\n")
	total := bd.ClaudeMD + bd.ToolCallIO + bd.MentionedFiles +
		bd.ExtendedThinking + bd.Conversation + bd.SkillActivations +
		bd.TeamOverhead + bd.UserText
	b.WriteString(fmt.Sprintf("  %-12s %s\n", "Total", formatTokens(total)))

	return b.String()
}

func formatTokens(n int) string {
	s := strconv.Itoa(n)
	cut := len(s) % 3
	if cut == 0 {
		cut = 3
	}
	var b strings.Builder
	b.WriteString(s[:cut])
	for i := cut; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
