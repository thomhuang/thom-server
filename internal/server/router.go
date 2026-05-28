package server

import (
	"net/http"

	coffeehttp "thom-server/internal/server/coffee"
)

func (app *App) routes() *http.ServeMux {
	mux := http.NewServeMux()
	coffeeHandler := coffeehttp.New(app.coffee, app.responder)

	mux.HandleFunc("POST /auth/login", app.auth.Login)
	mux.HandleFunc("POST /auth/logout", app.auth.RequireAuth(app.auth.Logout))
	mux.HandleFunc("GET /auth/me", app.auth.RequireAuth(app.auth.GetCurrentUser))

	mux.HandleFunc("GET /coffee", coffeeHandler.GetEntries)
	mux.HandleFunc("GET /coffee/roasters", coffeeHandler.GetRoasters)
	mux.HandleFunc("POST /coffee/roasters", app.auth.RequireAuth(coffeeHandler.CreateRoaster))
	mux.HandleFunc("GET /coffee/{id}", coffeeHandler.GetEntryByID)
	mux.HandleFunc("POST /coffee", app.auth.RequireAuth(coffeeHandler.CreateEntry))
	mux.HandleFunc("PATCH /coffee/{id}", app.auth.RequireAuth(coffeeHandler.UpdateEntry))
	mux.HandleFunc("DELETE /coffee/{id}", app.auth.RequireAuth(coffeeHandler.DeleteEntry))

	return mux
}
