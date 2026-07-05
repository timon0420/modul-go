package main

import (
	"testing"

	appanalysis "connect-to-mongodb/internal/analysis"
)

func TestUnreadNotifications(t *testing.T) {
	notifications := []appanalysis.Notification{
		{ID: "unread-1", Read: false},
		{ID: "read", Read: true},
		{ID: "unread-2", Read: false},
	}

	pending := unreadNotifications(notifications)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending notifications, got %d", len(pending))
	}
	if pending[0].ID != "unread-1" || pending[1].ID != "unread-2" {
		t.Fatalf("unexpected pending notifications: %#v", pending)
	}
}
