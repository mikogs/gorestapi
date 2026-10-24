package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi"
	_ "modernc.org/sqlite"
)

type User struct {
	ID         uint64
	Name       string `validate:"required,max=100"`
	Email      string `validate:"required,email" crud:"uniq"`
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

	reg := registry.New(map[string]func() any{
		"users": func() any { return &User{} },
		"notes": func() any { return &Note{} },
	}, db, gocrud.Options{Dialect: gocrud.DialectSQLite})

	// Migrate creates the tables, and adds the columns of new fields to existing ones
	if _, err := reg.Migrate(context.Background()); err != nil {
		slog.Error("failed to migrate tables", "error", err)
		os.Exit(1)
	}

	// X-User-ID is intentionally simple — a real app would use JWT or sessions.
	userIDFromRequest := func(r *http.Request) any {
		id, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
		return id
	}

	// noteComment stamps every create and update with a server-controlled comment.
	noteComment := func(obj any, _ *http.Request) error {
		obj.(*Note).Comment = "Added with API"
		return nil
	}

	// noteOwner rejects requests where X-User-ID does not match the note's UserID.
	noteOwner := func(obj any, r *http.Request) error {
		note := obj.(*Note)
		headerUserID, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
		if note.UserID != headerUserID {
			return errors.New("not the note owner")
		}
		return nil
	}

	handler := gorestapi.New(reg, gorestapi.Options{
		UserIDFunc: userIDFromRequest,
		Routes: map[string]gorestapi.Route{
			// The audit fields (CreatedAt, CreatedBy, ModifiedAt, ModifiedBy) are set by gocrud,
			// which ignores the values sent by clients.
			"users": {},
			"notes": {
				AllowedFilters: []string{"UserID"},
				AllowUpdate:    noteOwner,
				AllowDelete:    noteOwner,
				PreCreate:      noteComment,
				PreUpdate:      noteComment,
				PostRead: func(obj any, _ *http.Request) error {
					obj.(*Note).Comment = "Returned from gocrud"
					return nil
				},
				PostListItem: func(obj any, _ *http.Request) error {
					obj.(*Note).Comment = "Returned from gocrud"
					return nil
				},
				FilterList: func(r *http.Request) gorestapi.FilterSet {
					return gorestapi.FilterSet{
						Vals: map[string]string{"UserID": r.Header.Get("X-User-ID")},
					}
				},
				FilterRead: func(r *http.Request) gorestapi.FilterSet {
					return gorestapi.FilterSet{
						Vals: map[string]string{"UserID": r.Header.Get("X-User-ID")},
					}
				},
			},
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
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
