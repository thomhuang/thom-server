package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"thom-server/internal/data"
)

func TestHandleDataError(t *testing.T) {
	responder := Responder{}

	tests := []struct {
		name   string
		err    error
		status int
		want   bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name:   "no record",
			err:    data.ErrNoRecord,
			status: http.StatusNotFound,
			want:   true,
		},
		{
			name:   "server error",
			err:    errors.New("database unavailable"),
			status: http.StatusInternalServerError,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			got := responder.HandleDataError(rr, tt.err)
			if got != tt.want {
				t.Fatalf("handleDataError() = %t, want %t", got, tt.want)
			}
			if tt.err != nil && rr.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, rr.Code)
			}
			if tt.err == nil && rr.Code != http.StatusOK {
				t.Fatalf("expected recorder to remain untouched, got %d", rr.Code)
			}
		})
	}
}
