package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"thom-server/internal/coffee"
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

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		infoLog.Printf("Starting server on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorLog.Fatal(err)
		}
	}()

	<-shutdown

	infoLog.Println("stopping server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		errorLog.Fatal(err)
	}
	infoLog.Println("server stopped")
}
