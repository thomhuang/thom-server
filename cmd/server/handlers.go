package main

import (
	"net/http"
	"strconv"
	"sync"
	"thom-server/internal/models"
)

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

	err = app.writeJSON(w, http.StatusOK, categories, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}
}

func (app *application) GetPostsByCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		app.clientError(w, http.StatusMethodNotAllowed)

		return
	}

}

func (app *application) GetPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("category"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	app.httpMethodCheck(r.Method, http.MethodGet, w)

	post, err := app.Posts.GetPosts(id)

	err = app.writeJSON(w, http.StatusOK, post, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}
}

func (app *application) GetPostContentById(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("post"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	app.httpMethodCheck(r.Method, http.MethodGet, w)

	var wg sync.WaitGroup
	wg.Add(2)

	postChan, contentChan := make(chan *models.Post, 1), make(chan []*models.PostContent, 1)

	go func() {
		defer wg.Done()

		postRes, postErr := app.Posts.GetPostById(id)
		if postErr != nil {
			app.serverError(w, postErr)
		}

		postChan <- postRes
	}()

	go func() {
		defer wg.Done()

		contentRes, contentErr := app.Posts.GetPostContentById(id)
		if contentErr != nil {
			app.serverError(w, contentErr)
		}

		contentChan <- contentRes
	}()

	wg.Wait()

	close(postChan)
	close(contentChan)

	post := <-postChan

	for _, contentChunk := range <-contentChan {
		post.Content = append(post.Content, *contentChunk)
	}

	err = app.writeJSON(w, http.StatusOK, post, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}
}

func (app *application) GetPostContentByPathName(w http.ResponseWriter, r *http.Request) {
	pathName := r.URL.Query().Get("pathName")
	if len(pathName) < 1 {
		app.notFound(w)
		return
	}

	app.httpMethodCheck(r.Method, http.MethodGet, w)

	var wg sync.WaitGroup
	wg.Add(2)

	postChan, contentChan := make(chan *models.Post, 1), make(chan []*models.PostContent, 1)

	go func() {
		defer wg.Done()

		postRes, postErr := app.Posts.GetPostByPathName(pathName)
		if postErr != nil {
			app.serverError(w, postErr)
		}

		postChan <- postRes
	}()

	go func() {
		defer wg.Done()

		contentRes, contentErr := app.Posts.GetPostContentByPathName(pathName)
		if contentErr != nil {
			app.serverError(w, contentErr)
		}

		contentChan <- contentRes
	}()

	wg.Wait()

	close(postChan)
	close(contentChan)

	post := <-postChan

	for _, contentChunk := range <-contentChan {
		post.Content = append(post.Content, *contentChunk)
	}

	err := app.writeJSON(w, http.StatusOK, post, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}
}
