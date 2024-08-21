package main

import (
	"net/http"
	"strconv"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w)
		return
	}

	w.Write([]byte("Hello from Thom-Server"))
}

func (app *application) ContentPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		app.clientError(w, http.StatusMethodNotAllowed)

		return
	}

	w.Write([]byte("Post View Contents!"))
}

func (app *application) RegisterUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	app.clientError(w, http.StatusMethodNotAllowed)

	return
}

func (app *application) LoginUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	app.clientError(w, http.StatusMethodNotAllowed)

	return
}
