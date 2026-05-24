package server

import (
	"log"
	"net/http"

	coffeedata "thom-server/internal/coffee"
	postdata "thom-server/internal/posts"
	authhttp "thom-server/internal/server/auth"
	"thom-server/internal/server/response"
)

type App struct {
	errorLog  *log.Logger
	infoLog   *log.Logger
	posts     *postdata.Model
	coffee    *coffeedata.Model
	config    Config
	responder response.Responder
	auth      *authhttp.Handler
}

func New(errorLog, infoLog *log.Logger, postModel *postdata.Model, coffeeModel *coffeedata.Model, config Config) *App {
	app := &App{
		errorLog:  errorLog,
		infoLog:   infoLog,
		posts:     postModel,
		coffee:    coffeeModel,
		config:    config,
		responder: response.Responder{ErrorLog: errorLog},
	}
	app.auth = authhttp.New(config.authConfig(), app.responder, app.isAllowedOrigin)

	return app
}

func (app *App) Handler() http.Handler {
	return app.commonMiddleware(app.routes())
}
