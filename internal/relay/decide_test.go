package relay

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/alertmanager"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	alert := func(id, status string, start time.Time) alertmanager.Alert {
		return alertmanager.Alert{Fingerprint: id, Status: status, StartsAt: start}
	}
	a := alert("a", "firing", now.Add(-time.Hour))
	b := alert("b", "firing", now.Add(-time.Hour))
	resolvedA := a
	resolvedA.Status = "resolved"
	resolvedA.EndsAt = now
	p := func(status string, alerts ...alertmanager.Alert) alertmanager.Payload {
		return alertmanager.Payload{Status: status, Alerts: alerts}
	}
	group := func(alerts ...alertmanager.Alert) *Group {
		g := &Group{MessageID: 10, ReplyMessageID: 20, OpenedAt: now.Add(-time.Hour)}
		for _, a := range alerts {
			g.Alerts = append(g.Alerts, StoredAlert{Alert: a})
		}
		return g
	}
	tests := []struct {
		name    string
		stored  *Group
		payload alertmanager.Payload
		want    []ActionType
		silent  bool
	}{
		{"new firing", nil, p("firing", a), []ActionType{SendMessage, SaveGroup}, false},
		{"new resolved", nil, p("resolved", resolvedA), []ActionType{SendMessage}, true},
		{"new occurrence", group(a), p("firing", a, b), []ActionType{EditMessage, SendReply, DeleteReply, SaveGroup}, false},
		{"partial resolve", group(a, b), p("firing", resolvedA, b), []ActionType{EditMessage, SaveGroup}, false},
		{"repeat", group(a), p("firing", a), []ActionType{EditMessage, SendReply, DeleteReply, SaveGroup}, false},
		{"resolved", group(a), p("resolved", resolvedA), []ActionType{EditMessage, DeleteReply, DeleteGroup}, false},
		{"same fingerprint new start", group(a), p("firing", alert("a", "firing", now)), []ActionType{EditMessage, SendReply, DeleteReply, SaveGroup}, false},
		{"new and resolved", group(a), p("firing", resolvedA, b), []ActionType{EditMessage, SendReply, DeleteReply, SaveGroup}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := Decide(tt.stored, tt.payload, now, time.UTC)
			var got []ActionType
			for _, a := range actions {
				got = append(got, a.Type)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("actions = %v, want %v", got, tt.want)
			}
			if tt.stored == nil && actions[0].Silent != tt.silent {
				t.Errorf("silent = %v", actions[0].Silent)
			}

		})
	}
}

func TestSavedStateAndReturnedIDs(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := alertmanager.Alert{Fingerprint: "a", Status: "firing", StartsAt: now.Add(-time.Hour), Labels: map[string]string{"alertname": "A"}}
	b := alertmanager.Alert{Fingerprint: "b", Status: "firing", StartsAt: now.Add(-time.Hour), Labels: map[string]string{"alertname": "B"}}
	payload := alertmanager.Payload{Status: "firing", Alerts: []alertmanager.Alert{a, b}}
	actions := Decide(nil, payload, now, time.UTC)
	save := &actions[1]
	save.RecordReturnedID(SendMessage, 10)
	if save.Group.MessageID != 10 || len(save.Group.Alerts) != 2 {
		t.Fatalf("saved group: %+v", save.Group)
	}
	stored := save.Group
	firstRepeat := Decide(stored, payload, now, time.UTC)
	if len(firstRepeat) != 3 || firstRepeat[0].MessageID != 10 || firstRepeat[1].Type != SendReply || firstRepeat[1].MessageID != 10 || firstRepeat[2].Type != SaveGroup {
		t.Fatalf("first repeat: %+v", firstRepeat)
	}
	firstRepeat[2].RecordReturnedID(SendReply, 20)
	if firstRepeat[2].Group.ReplyMessageID != 20 || stored.ReplyMessageID != 0 {
		t.Fatal("reply ID not saved or input changed")
	}
	resolved := a
	resolved.Status, resolved.EndsAt = "resolved", now
	partial := Decide(firstRepeat[2].Group, alertmanager.Payload{Status: "firing", Alerts: []alertmanager.Alert{resolved, b}}, now, time.UTC)
	saved := partial[len(partial)-1].Group
	if saved.Alerts[0].ResolvedAt == nil || !saved.Alerts[0].ResolvedAt.Equal(now) || saved.Alerts[0].Alert.Labels["alertname"] != "A" {
		t.Fatalf("resolved state: %+v", saved.Alerts[0])
	}
	repeat := Decide(saved, alertmanager.Payload{Status: "firing", Alerts: []alertmanager.Alert{b}}, now.Add(time.Minute), time.UTC)
	if repeat[0].MessageID != 10 || repeat[1].MessageID != 10 || repeat[2].Type != DeleteReply || repeat[2].MessageID != 20 {
		t.Fatalf("targets: %+v", repeat)
	}
	if !strings.Contains(repeat[0].Text, "✅ A") || len(repeat[3].Group.Alerts) != 2 {
		t.Fatalf("resolved alert lost: %+v", repeat)
	}
	if stored.Alerts[0].Alert.Status != "firing" {
		t.Fatal("input alert changed")
	}
}

func TestStoredOccurrencesSurviveJSONAndMissingPayload(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	first := alertmanager.Alert{Fingerprint: "same", StartsAt: now.Add(-time.Hour), Status: "resolved", Labels: map[string]string{"alertname": "old"}}
	second := alertmanager.Alert{Fingerprint: "same", StartsAt: now, Status: "firing", Labels: map[string]string{"alertname": "new"}}
	resolved := now
	group := Group{OpenedAt: first.StartsAt, Alerts: []StoredAlert{{Alert: first, ResolvedAt: &resolved, MissedReads: 2}, {Alert: second, MissedReads: 1}}}
	data, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	var restored Group
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	actions := Decide(&restored, alertmanager.Payload{Status: "firing"}, now, time.UTC)
	saved := actions[len(actions)-1].Group
	if len(saved.Alerts) != 2 || saved.Alerts[0].MissedReads != 2 || saved.Alerts[1].MissedReads != 1 || saved.Alerts[0].ResolvedAt == nil || !saved.Alerts[0].ResolvedAt.Equal(now) {
		t.Fatalf("saved: %+v", saved.Alerts)
	}
}
