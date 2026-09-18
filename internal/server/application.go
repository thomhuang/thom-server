package server

import (
	"io"
	"log"
	"net/http"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/r2"
	authhttp "thom-server/internal/server/auth"
	"thom-server/internal/server/response"
	shophttp "thom-server/internal/server/shop"
	shopdata "thom-server/internal/shop"
)

type App struct {
	errorLog   *log.Logger
	infoLog    *log.Logger
	coffee     *coffeedata.Model
	shop       *shopdata.Model
	imageStore shophttp.ImageStore
	stripe     shophttp.StripeClient
	config     Config
	responder  response.Responder
	auth       *authhttp.Handler
}

func New(errorLog, infoLog *log.Logger, coffeeModel *coffeedata.Model, shopModel *shopdata.Model, config Config) *App {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}
	app := &App{
		errorLog:   errorLog,
		infoLog:    infoLog,
		coffee:     coffeeModel,
		shop:       shopModel,
		imageStore: r2.New(config.R2),
		stripe:     shophttp.NewStripeClient(config.Stripe.SecretKey),
		config:     config,
		responder:  response.Responder{ErrorLog: errorLog},
	}
	app.auth = authhttp.New(config.authConfig(), app.responder, app.isAllowedOrigin, app.infoLog)

	return app
}

func (app *App) Handler() http.Handler {
	return app.commonMiddleware(app.routes())
}

// ping is used by the Cloudflare Containers health check.
func (app *App) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
