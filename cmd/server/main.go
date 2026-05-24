package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"thom-server/internal/coffee"
	"thom-server/internal/posts"
	"thom-server/internal/server"
)

func main() {
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	if err := server.LoadLocalEnv(".env.local"); err != nil {
		errorLog.Fatal(err)
	}

	addr := flag.String("addr", ":4000", "HTTP network address")
	dbPath := flag.String("db-path", server.DBPathFromEnv(), "SQLite database path")

	flag.Parse()

	if err := server.PrepareDBFile(*dbPath, server.DefaultDBPath); err != nil {
		errorLog.Fatal(err)
	}

	appDB, err := server.OpenDB(*dbPath)
	if err != nil {
		errorLog.Fatal(err)
	}
	defer appDB.Close()

	coffeeModel := &coffee.Model{DB: appDB}
	if err := coffeeModel.EnsureSchema(); err != nil {
		errorLog.Fatal(err)
	}

	app := server.New(
		errorLog,
		infoLog,
		&posts.Model{DB: appDB},
		coffeeModel,
		server.Config{
			AdminUsername:     os.Getenv("ADMIN_USERNAME"),
			AdminPasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
			JWTSecret:         os.Getenv("JWT_SECRET"),
			ClientOrigins:     server.ClientOriginsFromEnv(),
			SecureCookies:     server.SecureCookiesFromEnv(),
		},
	)

	srv := &http.Server{
		Addr:              *addr,
		ErrorLog:          errorLog,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
	}

	infoLog.Printf("Starting server on %s", *addr)
	err = srv.ListenAndServe()
	errorLog.Fatal(err)
}
