package client

import "testing"

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
