package main

import (
	"fmt"
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

func (app *application) GetPostCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		app.clientError(w, http.StatusMethodNotAllowed)

		return
	}

	categories, err := app.Posts.GetCategories()
	if err != nil {
		app.serverError(w, err)
		return
	}

	for _, category := range categories {
		fmt.Fprintf(w, category.Category)
	}
}

func (app *application) GetPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		app.clientError(w, http.StatusMethodNotAllowed)

		return
	}

	post, err := app.Posts.GetPosts(id)
	if err != nil {
		app.serverError(w, err)
		return
	}

	fmt.Fprint(w, post.Title)
}
