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
	svccrud "github.com/mikogs/gocrud/pkg/service"
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

type Note struct {
	ID         uint64
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
	defer db.Close()

	svc := svccrud.New(map[string]func() interface{}{
		"users": func() interface{} { return &User{} },
		"notes": func() interface{} { return &Note{} },
	}, db, gocrud.DialectSQLite)

	// Migrate creates the tables, and adds the columns of new fields to existing ones
	if _, err := svc.Migrate(context.Background()); err != nil {
		slog.Error("failed to migrate tables", "error", err)
		os.Exit(1)
	}

	// X-User-ID is intentionally simple — a real app would use JWT or sessions.
	userIDFromRequest := func(r *http.Request) uint64 {
		id, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
		return id
	}

	// noteComment stamps every create and update with a server-controlled comment.
	noteComment := func(obj interface{}, _ *http.Request) error {
		obj.(*Note).Comment = "Added with API"
		return nil
	}

	// noteOwner rejects requests where X-User-ID does not match the note's UserID.
	noteOwner := func(obj interface{}, r *http.Request) error {
		note := obj.(*Note)
		headerUserID, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
		if note.UserID != headerUserID {
			return errors.New("not the note owner")
		}
		return nil
	}

	handler := gorestapi.New(svc, gorestapi.Options{
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
				PostRead: func(obj interface{}, _ *http.Request) error {
					obj.(*Note).Comment = "Returned from gocrud"
					return nil
				},
				PostListItem: func(obj interface{}, _ *http.Request) error {
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
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}

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
