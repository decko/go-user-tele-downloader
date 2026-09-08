package client

import (
	"testing"

	"github.com/gotd/td/tg"

	"github.com/decko/go-user-tele-downloader/internal/domain"
)

func TestQueueStateMessage(t *testing.T) {
	tests := []struct {
		name  string
		state domain.QueueState
		want  string
	}{
		{name: "empty", state: domain.QueueState{Queued: 0}, want: "⏳ Queue empty"},
		{name: "negative", state: domain.QueueState{Queued: -1}, want: "⏳ Queue empty"},
		{name: "one waiting", state: domain.QueueState{Queued: 1}, want: "⏳ Queue: 1 waiting"},
		{
			name:  "with items",
			state: domain.QueueState{Queued: 3, Items: []string{"a.mkv", "b.mkv", "c.mkv"}},
			want:  "⏳ Queue: 3 waiting\n\n• a.mkv\n• b.mkv\n• c.mkv",
		},
	}

	for _, tc := range tests {
		if got := queueStateMessage(tc.state); got != tc.want {
			t.Errorf("queueStateMessage(%+v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestExtractMessageID(t *testing.T) {
	resp := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 42},
		},
	}
	if got := extractMessageID(resp); got != 42 {
		t.Fatalf("extractMessageID = %d, want 42", got)
	}

	// A non-*tg.Updates response yields 0.
	if got := extractMessageID(&tg.UpdateShort{}); got != 0 {
		t.Fatalf("extractMessageID(short) = %d, want 0", got)
	}
}
