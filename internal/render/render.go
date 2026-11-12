package render

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/alezkv/alertmanager-telegram-relay/internal/alertmanager"
)

const Limit = 4096

// Message renders Telegram HTML. openedAt is the start of the incident.
func Message(p alertmanager.Payload, openedAt, now time.Time, loc *time.Location) string {
	icon := "🔥"
	if p.Status == "resolved" {
		icon = "✅"
	}
	labels := p.CommonLabels
	headerText := fmt.Sprintf("%s %s · %s · %s", icon, labels["alertname"], labels["severity"], labels["namespace"])
	resolvedText := ""
	if p.Status == "resolved" {
		resolved := time.Time{}
		for _, a := range p.Alerts {
			if a.EndsAt.After(resolved) {
				resolved = a.EndsAt
			}
		}
		if resolved.IsZero() {
			resolved = now
		}
		duration := resolved.Sub(openedAt)
		if openedAt.IsZero() || duration < 0 {
			duration = 0
		}
		resolvedText = fmt.Sprintf("\nResolved: %s · Duration: %s", resolved.In(loc).Format("2006-01-02 15:04 MST"), formatDuration(duration))
	}
	if p.TruncatedAlerts > 0 {
		resolvedText += fmt.Sprintf("\nAlertmanager omitted %d %s", p.TruncatedAlerts, pluralAlert(p.TruncatedAlerts))
	}
	// Reserve space for the omission count even when the header is very long.
	reserve := units(omission(len(p.Alerts)))
	if len(p.Alerts) == 0 {
		reserve = 0
	}
	headerText = truncate(headerText, Limit-reserve-units(resolvedText)) + resolvedText
	// Keep the alert name bold when it fits in the header.
	header := html.EscapeString(headerText)
	name := html.EscapeString(labels["alertname"])
	if name != "" && strings.HasPrefix(header, icon+" "+name) {
		header = icon + " <b>" + name + "</b>" + strings.TrimPrefix(header, icon+" "+name)
	}
	blocks := make([]string, len(p.Alerts))
	lengths := make([]int, len(p.Alerts))
	for i, a := range p.Alerts {
		prefix := ""
		if p.Status == "firing" && a.Status == "resolved" {
			prefix = "✅ "
		}
		keys := make([]string, 0, len(a.Labels))
		for key := range a.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		labelParts := make([]string, 0, len(keys))
		for _, key := range keys {
			labelParts = append(labelParts, key+"="+a.Labels[key])
		}
		text := prefix + a.Labels["alertname"]
		if len(labelParts) > 0 {
			text += "\n" + strings.Join(labelParts, ", ")
		}
		if a.Annotations["summary"] != "" {
			if text != "" && text != prefix {
				text += "\n"
			}
			text += a.Annotations["summary"]
		}
		if a.Annotations["description"] != "" {
			text += "\n" + a.Annotations["description"]
		}
		block := "\n\n" + html.EscapeString(text)
		visible := "\n\n" + text
		if a.GeneratorURL != "" {
			block += "\n<a href=\"" + html.EscapeString(a.GeneratorURL) + "\">source</a>"
			visible += "\nsource"
		}
		if a.Annotations["dashboard"] != "" {
			if a.GeneratorURL != "" {
				block += " · "
				visible += " · "
			} else {
				block += "\n"
				visible += "\n"
			}
			block += "<a href=\"" + html.EscapeString(a.Annotations["dashboard"]) + "\">dashboard</a>"
			visible += "dashboard"
		}
		blocks[i], lengths[i] = block, units(visible)
	}
	used, count := units(headerText), 0
	for i := range blocks {
		remaining := len(blocks) - i - 1
		suffix := ""
		if remaining > 0 {
			suffix = omission(remaining)
		}
		if used+lengths[i]+units(suffix) > Limit {
			break
		}
		header += blocks[i]
		used += lengths[i]
		count++
	}
	if count < len(blocks) {
		header += omission(len(blocks) - count)
	}
	return header
}

func units(s string) int { return len(utf16.Encode([]rune(s))) }
func truncate(s string, max int) string {
	if units(s) <= max {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		n := units(string(r))
		if max-n < 0 {
			break
		}
		b.WriteRune(r)
		max -= n
	}
	return b.String()
}

func pluralAlert(n int) string {
	if n == 1 {
		return "alert"
	}
	return "alerts"
}

func omission(n int) string { return fmt.Sprintf("\n… %d %s omitted", n, pluralAlert(n)) }

func formatDuration(d time.Duration) string {
	minutes := int64(d / time.Minute)
	return fmt.Sprintf("%dd %dh %dm", minutes/(24*60), minutes/60%24, minutes%60)
}
