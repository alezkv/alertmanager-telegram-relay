package relay

import (
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/render"
	"github.com/alezkv/alertmanager-telegram-relay/internal/webhook"
)

type Occurrence struct {
	Fingerprint string
	StartsAt    time.Time
}
type Group struct {
	MessageID      int64
	ReplyMessageID int64
	OpenedAt       time.Time
	Webhook        webhook.Payload
}

type ActionType string

const (
	SendMessage ActionType = "send_message"
	EditMessage ActionType = "edit_message"
	SendReply   ActionType = "send_reply"
	DeleteReply ActionType = "delete_reply"
	SaveGroup   ActionType = "save_group"
	DeleteGroup ActionType = "delete_group"
)

type Action struct {
	Type      ActionType
	MessageID int64
	Text      string
	Silent    bool
	Group     *Group
}

// Decide returns ordered actions without changing the input state.
func Decide(stored *Group, p webhook.Payload, now time.Time) []Action {
	opened := now
	if stored != nil {
		opened = stored.OpenedAt
	} else {
		found := false
		for _, a := range p.Alerts {
			if !a.StartsAt.IsZero() && (!found || a.StartsAt.Before(opened)) {
				opened = a.StartsAt
				found = true
			}
		}
	}
	message := render.Message(p, opened, now)
	if stored == nil {
		actions := []Action{{Type: SendMessage, Text: message, Silent: p.Status == "resolved"}}
		if p.Status != "resolved" {
			actions = append(actions, Action{Type: SaveGroup, Group: &Group{OpenedAt: opened, Webhook: p}})
		}
		return actions
	}
	actions := []Action{{Type: EditMessage, MessageID: stored.MessageID, Text: message}}
	if p.Status == "resolved" {
		if stored.ReplyMessageID != 0 {
			actions = append(actions, Action{Type: DeleteReply, MessageID: stored.ReplyMessageID})
		}
		return append(actions, Action{Type: DeleteGroup})
	}
	old := make(map[Occurrence]bool)
	for _, a := range stored.Webhook.Alerts {
		if a.Status == "firing" {
			old[Occurrence{a.Fingerprint, a.StartsAt.UTC()}] = true
		}
	}
	current := make(map[Occurrence]bool)
	for _, a := range p.Alerts {
		if a.Status == "firing" {
			current[Occurrence{a.Fingerprint, a.StartsAt.UTC()}] = true
		}
	}
	newAlert, removed := false, false
	for k := range current {
		if !old[k] {
			newAlert = true
		}
	}
	for k := range old {
		if !current[k] {
			removed = true
		}
	}
	if newAlert || !removed {
		actions = append(actions, Action{Type: SendReply, MessageID: stored.MessageID, Text: "🔥 Alert still firing"})
		if stored.ReplyMessageID != 0 {
			actions = append(actions, Action{Type: DeleteReply, MessageID: stored.ReplyMessageID})
		}
	}
	group := *stored
	group.Webhook = p
	return append(actions, Action{Type: SaveGroup, Group: &group})
}
