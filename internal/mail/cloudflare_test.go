package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCloudflareSenderDisabledWithoutConfig(t *testing.T) {
	testCases := []struct {
		name      string
		accountID string
		apiToken  string
		from      string
	}{
		{name: "missing account", apiToken: "token", from: "orders@example.com"},
		{name: "missing token", accountID: "acct", from: "orders@example.com"},
		{name: "missing from", accountID: "acct", apiToken: "token"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if sender := NewCloudflareSender(testCase.accountID, testCase.apiToken, testCase.from, ""); sender != nil {
				t.Fatalf("sender = %#v, want nil when config is incomplete", sender)
			}
		})
	}
}

func TestCloudflareSenderPostsToEmailSending(t *testing.T) {
	var (
		gotPath    string
		gotAuth    string
		gotType    string
		gotRequest cloudflareSendRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"errors":[],"result":{"delivered":["buyer@example.com"]}}`)
	}))
	defer server.Close()

	sender := &CloudflareSender{
		accountID: "acct",
		apiToken:  "secret-token",
		from:      "orders@example.com",
		fromName:  "Thom Huang",
		baseURL:   server.URL,
		client:    server.Client(),
	}

	err := sender.Send(context.Background(), Message{
		To:      "buyer@example.com",
		Subject: "Your order #7",
		Text:    "Thanks",
		HTML:    "<p>Thanks</p>",
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/accounts/acct/email/sending/send" {
		t.Fatalf("path = %q, want the email sending endpoint", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("authorization = %q, want the bearer token", gotAuth)
	}
	if gotType != "application/json" {
		t.Fatalf("content type = %q, want application/json", gotType)
	}
	if gotRequest.To != "buyer@example.com" || gotRequest.Subject != "Your order #7" {
		t.Fatalf("request = %+v, want the recipient and subject", gotRequest)
	}
	if gotRequest.From.Address != "orders@example.com" || gotRequest.From.Name != "Thom Huang" {
		t.Fatalf("from = %+v, want the configured sender", gotRequest.From)
	}
	if gotRequest.Text != "Thanks" || gotRequest.HTML != "<p>Thanks</p>" {
		t.Fatalf("body = %+v, want both text and html", gotRequest)
	}
}

func TestCloudflareSenderReportsAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10102,"message":"email.sending.error.authentication.forbidden"}]}`)
	}))
	defer server.Close()

	sender := &CloudflareSender{
		accountID: "acct",
		apiToken:  "token",
		from:      "orders@example.com",
		baseURL:   server.URL,
		client:    server.Client(),
	}

	err := sender.Send(context.Background(), Message{To: "buyer@example.com", Subject: "Hi", Text: "Hi"})
	if err == nil {
		t.Fatal("expected an error for a failed send")
	}
}

func TestCloudflareSenderReportsSuccessFalse(t *testing.T) {
	// A 200 response can still carry success=false, so the body must be checked
	// rather than only the status code.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10001,"message":"email.sending.error.invalid_request_schema"}]}`)
	}))
	defer server.Close()

	sender := &CloudflareSender{
		accountID: "acct",
		apiToken:  "token",
		from:      "orders@example.com",
		baseURL:   server.URL,
		client:    server.Client(),
	}

	if err := sender.Send(context.Background(), Message{To: "buyer@example.com", Subject: "Hi", Text: "Hi"}); err == nil {
		t.Fatal("expected an error when success is false")
	}
}
