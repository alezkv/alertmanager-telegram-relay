package render

import (
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/webhook"
)

func TestMessage(t *testing.T) {
	now := time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC)
	p := webhook.Payload{Status: "firing", CommonLabels: map[string]string{"alertname": "<alert>", "severity": "a&b", "namespace": "ns"}, Alerts: []webhook.Alert{{Status: "resolved", Annotations: map[string]string{"summary": "<summary> &", "description": "\"detail\"", "dashboard": "https://example.test/?a=1&b=2"}, GeneratorURL: "https://example.test/?x=1&y=2"}}}
	got := Message(p, now.Add(-time.Hour), now)
	for _, s := range []string{"🔥 <b>&lt;alert&gt;</b> · a&amp;b · ns", "✅ &lt;summary&gt; &amp;", "&#34;detail&#34;", "?x=1&amp;y=2", "?a=1&amp;b=2"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in %s", s, got)
		}
	}
	p.Status = "resolved"
	p.Alerts[0].EndsAt = now
	got = Message(p, now.Add(-time.Hour), now)
	for _, s := range []string{"✅ <b>", "2026-01-02 12:00 CET", "Duration: 1h0m0s"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in %s", s, got)
		}
	}
}

func TestLimit(t *testing.T) {
	tags := regexp.MustCompile(`<[^>]*>`)
	cases := []struct {
		name    string
		p       webhook.Payload
		omitted string
	}{
		{"whole blocks", webhook.Payload{Status: "firing", Alerts: []webhook.Alert{{Annotations: map[string]string{"summary": strings.Repeat("a", 2000)}}, {Annotations: map[string]string{"summary": strings.Repeat("b", 2100)}}, {Annotations: map[string]string{"summary": "last"}}}}, "2 alerts omitted"},
		{"large header", webhook.Payload{Status: "firing", CommonLabels: map[string]string{"alertname": strings.Repeat("😀", 5000)}, Alerts: []webhook.Alert{{Annotations: map[string]string{"summary": strings.Repeat("b", 100)}}}}, "1 alerts omitted"},
		{"large block", webhook.Payload{Status: "firing", Alerts: []webhook.Alert{{Annotations: map[string]string{"summary": strings.Repeat("&", 5000)}}}}, "1 alerts omitted"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Message(tt.p, time.Time{}, time.Now())
			visible := html.UnescapeString(tags.ReplaceAllString(got, ""))
			if units(visible) > Limit {
				t.Errorf("length = %d", units(visible))
			}
			if !strings.Contains(got, tt.omitted) {
				t.Errorf("missing omission count: %s", got)
			}
			if strings.Contains(got, "last") || strings.Contains(got, strings.Repeat("b", 100)) {
				t.Error("included part of an omitted block")
			}
		})
	}
}
