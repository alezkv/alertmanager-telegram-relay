package relay

import (
	"reflect"
	"testing"
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/webhook"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	alert := func(id, status string, start time.Time) webhook.Alert {
		return webhook.Alert{Fingerprint: id, Status: status, StartsAt: start}
	}
	a := alert("a", "firing", now.Add(-time.Hour))
	b := alert("b", "firing", now.Add(-time.Hour))
	resolvedA := a
	resolvedA.Status = "resolved"
	resolvedA.EndsAt = now
	p := func(status string, alerts ...webhook.Alert) webhook.Payload {
		return webhook.Payload{Status: status, Alerts: alerts}
	}
	group := func(alerts ...webhook.Alert) *Group {
		return &Group{MessageID: 10, ReplyMessageID: 20, OpenedAt: now.Add(-time.Hour), Webhook: p("firing", alerts...)}
	}
	tests := []struct {
		name    string
		stored  *Group
		payload webhook.Payload
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
			actions := Decide(tt.stored, tt.payload, now)
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
			if tt.stored != nil && tt.stored.Webhook.Status != "firing" {
				t.Fatal("stored group changed")
			}
		})
	}
}
