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

	"thom-server/internal/authstate"
	"thom-server/internal/blog"
	"thom-server/internal/coffee"
	"thom-server/internal/server"
	"thom-server/internal/shop"
)

func main() {
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	if err := server.LoadLocalEnv(".env.local"); err != nil {
		errorLog.Fatal(err)
	}

	addr := flag.String("addr", ":4000", "HTTP network address")

	flag.Parse()

	appDB, err := server.OpenAppDB()
	if err != nil {
		errorLog.Fatal(err)
	}
	defer appDB.Close()

	blogModel := &blog.Model{DB: appDB}
	if err := blogModel.EnsureSchema(); err != nil {
		errorLog.Fatal(err)
	}

	coffeeModel := &coffee.Model{DB: appDB}
	if err := coffeeModel.EnsureSchema(); err != nil {
		errorLog.Fatal(err)
	}

	shopModel := &shop.Model{DB: appDB}
	if err := shopModel.EnsureSchema(); err != nil {
		errorLog.Fatal(err)
	}

	authState := &authstate.Model{DB: appDB}
	if err := authState.EnsureSchema(); err != nil {
		errorLog.Fatal(err)
	}

	clientOrigins := server.ClientOriginsFromEnv()

	r2Config := server.R2ConfigFromEnv()
	if err := r2Config.Validate(); err != nil {
		errorLog.Printf("R2 image uploads will fail: %v", err)
	}

	config := server.Config{
		AdminUsername:     os.Getenv("ADMIN_USERNAME"),
		AdminPasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		ClientOrigins:     clientOrigins,
		SecureCookies:     server.SecureCookiesFromEnv(),
		R2:                r2Config,
		R2PublicBaseURL:   server.R2PublicBaseURLFromEnv(),
		Stripe:            server.StripeConfigFromEnv(clientOrigins),
		Email:             server.EmailConfigFromEnv(clientOrigins),
	}
	if err := config.Validate(); err != nil {
		errorLog.Fatal(err)
	}

	app := server.New(
		errorLog,
		infoLog,
		blogModel,
		coffeeModel,
		shopModel,
		config,
		authState,
	)

	// Expired checkout holds are released on a timer so an abandoned checkout
	// frees its item even when no webhook is delivered and no new checkout
	// arrives to trigger the lazy sweep.
	sweeperCtx, stopSweeper := context.WithCancel(context.Background())
	defer stopSweeper()
	go app.RunReservationSweeper(sweeperCtx)

	// Pasted post images that were never saved are reclaimed on the same timer,
	// so an abandoned blog draft cannot leave objects in R2 forever.
	go app.RunBlogUploadSweeper(sweeperCtx)

	srv := &http.Server{
		Addr:              *addr,
		ErrorLog:          errorLog,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// Must stay above the D1/R2 client timeouts (30s) so a slow-but-valid
		// outbound call is not aborted after a write may have committed.
		WriteTimeout: 40 * time.Second,
		IdleTimeout:  time.Minute,
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
