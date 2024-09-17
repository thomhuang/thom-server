package main

import "net/http"

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", app.home)

	// Content Handling
	mux.HandleFunc("/content/categories", app.GetPostCategories)
	mux.HandleFunc("/content/post", app.GetPost)

	return mux
}
