package main

import "net/http"

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", app.home)

	// Content Handling
	mux.HandleFunc(("/content/posts", app.ContentPost))

	// User Handling
	mux.HandleFunc("/user/register", app.RegisterUser)
	mux.HandleFunc("/user/login", app.LoginUser)

	return mux
}
