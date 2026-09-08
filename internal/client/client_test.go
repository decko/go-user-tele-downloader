package client

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestQueueStateMessage(t *testing.T) {
	tests := []struct {
		queued int64
		want   string
	}{
		{0, "Queue empty"},
		{-1, "Queue empty"},
		{1, "Queue: 1 waiting"},
		{5, "Queue: 5 waiting"},
	}

	for _, tc := range tests {
		if got := queueStateMessage(tc.queued); got != tc.want {
			t.Errorf("queueStateMessage(%d) = %q, want %q", tc.queued, got, tc.want)
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
