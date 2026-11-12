package render

import (
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/alertmanager"
)

func TestMessage(t *testing.T) {
	now := time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC)
	p := alertmanager.Payload{Status: "firing", CommonLabels: map[string]string{"alertname": "<alert>", "severity": "a&b", "namespace": "ns"}, Alerts: []alertmanager.Alert{{Status: "resolved", Annotations: map[string]string{"summary": "<summary> &", "description": "\"detail\"", "dashboard": "https://example.test/?a=1&b=2"}, GeneratorURL: "https://example.test/?x=1&y=2"}}}
	got := Message(p, now.Add(-time.Hour), now, time.FixedZone("CET", 3600))
	for _, s := range []string{"🔥 <b>&lt;alert&gt;</b> · a&amp;b · ns", "✅ &lt;summary&gt; &amp;", "&#34;detail&#34;", "?x=1&amp;y=2", "?a=1&amp;b=2"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in %s", s, got)
		}
	}
	p.Status = "resolved"
	p.Alerts[0].EndsAt = now
	got = Message(p, now.Add(-time.Hour), now, time.FixedZone("CET", 3600))
	for _, s := range []string{"✅ <b>", "2026-01-02 12:00 CET", "Duration: 0d 1h 0m"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in %s", s, got)
		}
	}
}

func TestLimit(t *testing.T) {
	tags := regexp.MustCompile(`<[^>]*>`)
	cases := []struct {
		name    string
		p       alertmanager.Payload
		omitted string
	}{
		{"whole blocks", alertmanager.Payload{Status: "firing", Alerts: []alertmanager.Alert{{Annotations: map[string]string{"summary": strings.Repeat("a", 2000)}}, {Annotations: map[string]string{"summary": strings.Repeat("b", 2100)}}, {Annotations: map[string]string{"summary": "last"}}}}, "2 alerts omitted"},
		{"large header", alertmanager.Payload{Status: "firing", CommonLabels: map[string]string{"alertname": strings.Repeat("😀", 5000)}, Alerts: []alertmanager.Alert{{Annotations: map[string]string{"summary": strings.Repeat("b", 100)}}}}, "1 alert omitted"},
		{"large block", alertmanager.Payload{Status: "firing", Alerts: []alertmanager.Alert{{Annotations: map[string]string{"summary": strings.Repeat("&", 5000)}}}}, "1 alert omitted"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Message(tt.p, time.Time{}, time.Now(), time.UTC)
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

func TestAlertLabelsDurationAndTruncation(t *testing.T) {
	now := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)
	loc := time.FixedZone("CEST", 2*60*60)
	p := alertmanager.Payload{Status: "resolved", TruncatedAlerts: 1, Alerts: []alertmanager.Alert{
		{Status: "resolved", EndsAt: now, Labels: map[string]string{"alertname": "First", "instance": "one"}},
		{Status: "resolved", EndsAt: now, Labels: map[string]string{"alertname": "Second", "instance": "two"}},
	}}
	got := Message(p, now.Add(-49*time.Hour-2*time.Minute), now, loc)
	for _, want := range []string{"2026-07-02 13:00 CEST", "Duration: 2d 1h 2m", "Alertmanager omitted 1 alert", "First\nalertname=First, instance=one", "Second\nalertname=Second, instance=two"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
