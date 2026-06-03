package response

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"thom-server/internal/data"
)

func TestWriteJSON(t *testing.T) {
	responder := Responder{}
	rr := httptest.NewRecorder()
	headers := http.Header{"X-Test": []string{"yes"}}

	err := responder.WriteJSON(rr, http.StatusCreated, map[string]string{"message": "ok"}, headers)
	if err != nil {
		t.Fatal(err)
	}

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}
	if got := rr.Header().Get("X-Test"); got != "yes" {
		t.Fatalf("expected custom header, got %q", got)
	}
	if !strings.HasSuffix(rr.Body.String(), "\n") {
		t.Fatal("expected response body to end with a newline")
	}

	var body map[string]string
	if err = json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "ok" {
		t.Fatalf("expected JSON body message ok, got %q", body["message"])
	}
}

func TestWriteJSONMarshalError(t *testing.T) {
	responder := Responder{}
	rr := httptest.NewRecorder()

	err := responder.WriteJSON(rr, http.StatusOK, make(chan int), nil)
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected no response body, got %q", rr.Body.String())
	}
}

func TestErrorHelpers(t *testing.T) {
	responder := Responder{ErrorLog: log.New(io.Discard, "", 0)}

	tests := []struct {
		name   string
		write  func(http.ResponseWriter)
		status int
	}{
		{
			name: "bad request",
			write: func(w http.ResponseWriter) {
				responder.BadRequest(w)
			},
			status: http.StatusBadRequest,
		},
		{
			name: "not found",
			write: func(w http.ResponseWriter) {
				responder.NotFound(w)
			},
			status: http.StatusNotFound,
		},
		{
			name: "server error",
			write: func(w http.ResponseWriter) {
				responder.ServerError(w, errors.New("boom"))
			},
			status: http.StatusInternalServerError,
		},
	}

		for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()

			tt.write(rr)

			if rr.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), http.StatusText(tt.status)) {
				t.Fatalf("expected body to contain status text %q, got %q", http.StatusText(tt.status), rr.Body.String())
			}
		})
	}
}

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
