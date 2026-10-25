package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/otelhook"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi"
	"github.com/mikogs/gorestapi/pkg/jwtauth"
	"github.com/rs/cors"
	_ "modernc.org/sqlite"
)

type User struct {
	ID         uint64
	Name       string `validate:"required,max=100"`
	Email      string `validate:"required,email" crud:"uniq"`
	Password   string `validate:"required,min=8,max=72" crud:"pass"`
	CreatedAt  time.Time
	CreatedBy  uint64
	ModifiedAt time.Time
	ModifiedBy uint64
}

// Note has a UUID, so that the URLs of notes do not reveal how many there are.
type Note struct {
	ID         string
	Title      string `validate:"required,max=200"`
	Content    string
	Comment    string
	UserID     uint64 `validate:"required" crud:"idx fk:user on_delete:cascade"`
	CreatedAt  time.Time
	CreatedBy  uint64
	ModifiedAt time.Time
	ModifiedBy uint64
}

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "poc.db"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// SQLite checks foreign keys only when they are enabled for the connection
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// the logs are JSON, with the IDs of the request, the trace and the span of each record of a request
	logger := slog.New(gorestapi.LogHandler(slog.NewJSONHandler(os.Stderr, nil)))
	slog.SetDefault(logger)

	metrics, shutdownTelemetry, err := setupTelemetry(context.Background(), db)
	if err != nil {
		slog.Error("failed to set up telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	queryHook, err := otelhook.New(otelhook.Options{Dialect: gocrud.DialectSQLite})
	if err != nil {
		slog.Error("failed to create the query hook", "error", err)
		os.Exit(1)
	}

	reg := registry.New(map[string]func() any{
		"users": func() any { return &User{} },
		"notes": func() any { return &Note{} },
	}, db, gocrud.Options{Dialect: gocrud.DialectSQLite, QueryHook: queryHook}).WithLogger(logger)

	// Migrate creates the tables, and adds the columns of new fields to existing ones
	if _, err := reg.Migrate(context.Background()); err != nil {
		slog.Error("failed to migrate tables", "error", err)
		os.Exit(1)
	}

	// the key of the tokens is new at every start, so that the demo needs no secrets, and its tokens are valid until it
	// stops; a real application loads it, e.g. with jwtauth.ParsePrivateKeyPEM
	signingKey, err := jwtauth.GenerateKey()
	if err != nil {
		slog.Error("failed to generate the key of the tokens", "error", err)
		os.Exit(1)
	}
	provider, err := jwtauth.NewProvider("http://localhost:"+port, signingKey)
	if err != nil {
		slog.Error("failed to create the provider of the tokens", "error", err)
		os.Exit(1)
	}

	userIDFromRequest := func(r *http.Request) any {
		return userID(r)
	}

	// noteSave stamps every create and update with a server-controlled comment, and with the user of the token, so that
	// a note cannot be given to another user.
	noteSave := func(obj any, r *http.Request) error {
		note := obj.(*Note)
		note.Comment = "Added with API"
		note.UserID = userID(r)
		return nil
	}

	// noteOwner rejects the requests of other users than the owner of the note.
	noteOwner := func(obj any, r *http.Request) error {
		if obj.(*Note).UserID != userID(r) {
			return errors.New("not the note owner")
		}
		return nil
	}

	// a user can see, change or delete only themselves, so that the e-mails are not public
	self := func(obj any, r *http.Request) error {
		if obj.(*User).ID != userID(r) {
			return errors.New("not the user")
		}
		return nil
	}
	selfOnly := func(r *http.Request) gorestapi.FilterSet {
		return gorestapi.FilterSet{Vals: map[string]string{"ID": strconv.FormatUint(userID(r), 10)}}
	}

	ownNotes := func(r *http.Request) gorestapi.FilterSet {
		return gorestapi.FilterSet{Vals: map[string]string{"UserID": strconv.FormatUint(userID(r), 10)}}
	}

	handler := gorestapi.New(reg, gorestapi.Options{
		UserIDFunc:   userIDFromRequest,
		Authenticate: authenticate(provider),
		// 10 requests a second of each client, with bursts of 20
		RateLimiter: gorestapi.NewRateLimiter(10, 20),
		Logger:      logger,
		AccessLog:   true,
		Routes: map[string]gorestapi.Route{
			// The audit fields (CreatedAt, CreatedBy, ModifiedAt, ModifiedBy) are set by gocrud,
			// which ignores the values sent by clients. Anyone can sign up, without a token.
			"users": {Anonymous: true, AllowUpdate: self, AllowDelete: self, FilterList: selfOnly, FilterRead: selfOnly},
			"notes": {
				AllowedFilters: []string{"UserID"},
				AllowUpdate:    noteOwner,
				AllowDelete:    noteOwner,
				PreCreate:      noteSave,
				PreUpdate:      noteSave,
				PostRead: func(obj any, _ *http.Request) error {
					obj.(*Note).Comment = "Returned from gocrud"
					return nil
				},
				PostListItem: func(obj any, _ *http.Request) error {
					obj.(*Note).Comment = "Returned from gocrud"
					return nil
				},
				FilterList: ownNotes,
				FilterRead: ownNotes,
			},
		},
	})

	// the browsers of the origin of CORS_ORIGIN, e.g. of a web app, can call the API, and read its headers
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3000"
	}
	api := cors.New(cors.Options{
		AllowedOrigins: []string{corsOrigin},
		AllowedMethods: []string{http.MethodGet, http.MethodPut, http.MethodDelete},
		AllowedHeaders: []string{"Authorization", "Content-Type", gorestapi.RequestIDHeader},
		ExposedHeaders: []string{gorestapi.RequestIDHeader, "Allow", "Retry-After"},
		MaxAge:         600,
	}).Handler(http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))

	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	// 1 login a second of each client, with bursts of 5, against guessing the passwords
	mux.Handle("/token", tokenHandler(reg, provider, gorestapi.NewRateLimiter(1, 5)))
	mux.HandleFunc("/healthz", gorestapi.Liveness)
	mux.Handle("/readyz", handler.Readiness(db.PingContext))
	mux.Handle("/metrics", metrics)

	spec, err := handler.OpenAPI(gorestapi.OpenAPIInfo{Title: "gorestapi PoC", Version: "1.0.0", Servers: []string{"/api"}, BearerFormat: "JWT"})
	if err != nil {
		slog.Error("failed to generate the OpenAPI document", "error", err)
		os.Exit(1)
	}
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(spec)
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerUI))
	})

	// the timeouts keep slow clients from holding connections open, e.g. by sending the headers byte by byte
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting server", "port", port, "db", dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slog.Info("shutting down")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

// swaggerUI is the page of Swagger UI of the OpenAPI document, with its scripts from a CDN.
const swaggerUI = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>gorestapi PoC</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.33.1/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.33.1/swagger-ui-bundle.js"></script>
  <script>SwaggerUIBundle({url: "/openapi.json", dom_id: "#swagger-ui"});</script>
</body>
</html>
`
