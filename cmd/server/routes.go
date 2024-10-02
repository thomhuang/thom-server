package main

import "net/http"

func commonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		next.ServeHTTP(w, r)
	})
}

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Content Handling
	mux.HandleFunc("/categories", app.GetPostCategories)
	mux.HandleFunc("/post", app.GetPost)
	mux.HandleFunc("/post/content/id", app.GetPostContentById)
	mux.HandleFunc("/post/content/path", app.GetPostContentByPathName)

	return mux
}
