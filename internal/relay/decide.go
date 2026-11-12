package relay

import (
	"time"

	"github.com/alezkv/alertmanager-telegram-relay/internal/alertmanager"
	"github.com/alezkv/alertmanager-telegram-relay/internal/render"
)

type Occurrence struct {
	Fingerprint string
	StartsAt    time.Time
}

type StoredAlert struct {
	Alert       alertmanager.Alert `json:"alert"`
	ResolvedAt  *time.Time         `json:"resolved_at"`
	MissedReads int                `json:"missed_reads"`
}

type Group struct {
	MessageID      int64             `json:"message_id"`
	ReplyMessageID int64             `json:"reply_message_id"`
	OpenedAt       time.Time         `json:"opened_at"`
	CommonLabels   map[string]string `json:"common_labels"`
	Alerts         []StoredAlert     `json:"alerts"`
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

// RecordReturnedID copies each successful send's ID into the SaveGroup action.
// The handler must call it before it executes SaveGroup.
func (a *Action) RecordReturnedID(send ActionType, id int64) {
	if a.Type != SaveGroup || a.Group == nil {
		return
	}
	switch send {
	case SendMessage:
		a.Group.MessageID = id
	case SendReply:
		a.Group.ReplyMessageID = id
	}
}

func occurrence(a alertmanager.Alert) Occurrence {
	return Occurrence{a.Fingerprint, a.StartsAt.UTC()}
}

// Decide returns ordered actions without changing the input state.
func Decide(stored *Group, p alertmanager.Payload, now time.Time, loc *time.Location) []Action {
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
	group := Group{OpenedAt: opened}
	if stored != nil {
		group = *stored
		group.Alerts = append([]StoredAlert(nil), stored.Alerts...)
	}
	group.CommonLabels = p.CommonLabels
	old := make(map[Occurrence]bool)
	index := make(map[Occurrence]int)
	for i, a := range group.Alerts {
		key := occurrence(a.Alert)
		index[key] = i
		if a.ResolvedAt == nil && a.Alert.Status == "firing" {
			old[key] = true
		}
	}
	current := make(map[Occurrence]bool)
	for _, a := range p.Alerts {
		key := occurrence(a)
		if a.Status == "firing" {
			current[key] = true
		}
		entry := StoredAlert{Alert: a}
		if a.Status == "resolved" {
			resolved := a.EndsAt
			if resolved.IsZero() {
				resolved = now
			}
			entry.ResolvedAt = &resolved
		}
		if i, ok := index[key]; ok {
			// A stale firing payload must not erase a recorded resolution.
			if group.Alerts[i].ResolvedAt != nil && entry.ResolvedAt == nil {
				continue
			}
			group.Alerts[i] = entry
		} else {
			index[key] = len(group.Alerts)
			group.Alerts = append(group.Alerts, entry)
		}
	}
	merged := p
	merged.Alerts = make([]alertmanager.Alert, 0, len(group.Alerts))
	for _, a := range group.Alerts {
		merged.Alerts = append(merged.Alerts, a.Alert)
	}
	message := render.Message(merged, opened, now, loc)
	if stored == nil {
		actions := []Action{{Type: SendMessage, Text: message, Silent: p.Status == "resolved"}}
		if p.Status != "resolved" {
			actions = append(actions, Action{Type: SaveGroup, Group: &group})
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
	return append(actions, Action{Type: SaveGroup, Group: &group})
}
