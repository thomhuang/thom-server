package main

import (
	"flag"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net/http"
	"os"
	"thom-server/internal/models"
)

// We want to allow different forms of logs, namely:
// - Errors
// - Infos
type application struct {
	errorLog *log.Logger
	infoLog  *log.Logger
	Posts    *models.PostModel
}

func main() {
	// When running from CLI, can define port of interest
	addr := flag.String("addr", ":4000", "HTTP network address")

	// read CLI args ...
	flag.Parse()

	// Create a logger for writing info messages ...
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	// Create a logger for writing error messages ...
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Connect to Posts DB
	postsDB, postsErr := openDB("posts.db")
	if postsErr != nil {
		errorLog.Fatal(postsErr)
	}
	defer postsDB.Close()

	// Initialize our application
	app := &application{
		errorLog: errorLog,
		infoLog:  infoLog,
		Posts:    &models.PostModel{DB: postsDB},
	}

	// Initialize our HTTP server with:
	// - Port Address of interest
	// - our Error Logger
	// - Our Routes + their Handlers
	srv := &http.Server{
		Addr:     *addr,
		ErrorLog: errorLog,
		Handler:  app.routes(),
	}

	infoLog.Printf("Starting server on %s", *addr)
	err := srv.ListenAndServe()
	errorLog.Fatal(err)
}
