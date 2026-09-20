package shop

import (
	"io"
	"log"
	"strings"
	"time"

	"thom-server/internal/mail"
	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

// ImageStore is an interface so handler tests can run without R2 credentials.
type ImageStore interface {
	PresignPut(objectKey, contentType string, expires time.Duration) (string, error)
	Delete(objectKey string) error
}

type Handler struct {
	shop              *shopdata.Model
	images            ImageStore
	publicURL         string
	stripe            StripeClient
	stripeSettings    StripeSettings
	mailer            mail.Sender
	siteURL           string
	notificationEmail string
	responder         response.Responder
	infoLog           *log.Logger
}

func New(model *shopdata.Model, images ImageStore, publicURL string, responder response.Responder, infoLog *log.Logger) *Handler {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}

	return &Handler{
		shop:      model,
		images:    images,
		publicURL: strings.TrimSuffix(strings.TrimSpace(publicURL), "/"),
		responder: responder,
		infoLog:   infoLog,
	}
}
