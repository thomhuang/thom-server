// Package mail sends transactional email. The only implementation talks to the
// Cloudflare Email Service REST API, but the interface keeps handlers testable
// without network access.
package mail

import "context"

// Message is one transactional email. Text is set as well as HTML so a client
// that does not render HTML still shows the content, and so spam filters have a
// plain-text part to score.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers one transactional email.
type Sender interface {
	Send(ctx context.Context, message Message) error
}
