package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"
	cloudflareSendPath   = "/accounts/%s/email/sending/send"
	// maxErrorBody bounds how much of a failed response is read into memory.
	maxErrorBody = 4 << 10
	sendTimeout  = 15 * time.Second
)

// CloudflareSender sends through the Email Service REST API, which is the right
// fit for the Go container: it is a plain HTTPS call, unlike the Worker
// send_email binding. See
// https://developers.cloudflare.com/email-service/api/send-emails/rest-api/.
type CloudflareSender struct {
	accountID string
	apiToken  string
	from      string
	fromName  string
	baseURL   string
	client    *http.Client
}

// NewCloudflareSender returns a sender, or nil when the account, token, or from
// address is missing, so the caller can leave order email disabled like an
// unconfigured Stripe client.
func NewCloudflareSender(accountID, apiToken, from, fromName string) Sender {
	accountID = strings.TrimSpace(accountID)
	apiToken = strings.TrimSpace(apiToken)
	from = strings.TrimSpace(from)
	if accountID == "" || apiToken == "" || from == "" {
		return nil
	}

	return &CloudflareSender{
		accountID: accountID,
		apiToken:  apiToken,
		from:      from,
		fromName:  strings.TrimSpace(fromName),
		baseURL:   cloudflareAPIBaseURL,
		client:    &http.Client{Timeout: sendTimeout},
	}
}

type cloudflareAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type cloudflareSendRequest struct {
	To      string            `json:"to"`
	From    cloudflareAddress `json:"from"`
	Subject string            `json:"subject"`
	Text    string            `json:"text"`
	HTML    string            `json:"html,omitempty"`
}

type cloudflareSendResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Send posts one email. A non-2xx response, or a 2xx with success=false, is an
// error; the API reports delivery problems both ways.
func (s *CloudflareSender) Send(ctx context.Context, message Message) error {
	payload, err := json.Marshal(cloudflareSendRequest{
		To:      message.To,
		From:    cloudflareAddress{Address: s.from, Name: s.fromName},
		Subject: message.Subject,
		Text:    message.Text,
		HTML:    message.HTML,
	})
	if err != nil {
		return err
	}

	url := s.baseURL + fmt.Sprintf(cloudflareSendPath, s.accountID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.apiToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))

	var parsed cloudflareSendResponse
	_ = json.Unmarshal(body, &parsed)

	if response.StatusCode < 200 || response.StatusCode >= 300 || !parsed.Success {
		return fmt.Errorf(
			"mail: cloudflare send failed with status %d: %s",
			response.StatusCode,
			summarizeErrors(parsed.Errors, body),
		)
	}

	return nil
}

func summarizeErrors(errors []cloudflareError, body []byte) string {
	if len(errors) == 0 {
		return strings.TrimSpace(string(body))
	}

	parts := make([]string, 0, len(errors))
	for _, cloudflareErr := range errors {
		parts = append(parts, fmt.Sprintf("%d %s", cloudflareErr.Code, cloudflareErr.Message))
	}

	return strings.Join(parts, "; ")
}
