# Demo

The `cmd/poc` directory contains a self-contained runnable server that demonstrates the most common gorestapi patterns: two resources, a create-only struct, ownership guards, and server-side filters. The walkthrough below follows that code.

## Resources

Two structs are defined — `User` and `Note`. Each has a paired `_Draft` type used as the create payload; gocrud strips the underscore suffix to map it to the same table.

```go
type User struct {
    ID         uint64
    Name       string `crud:"req len:1,100"`
    Email      string `crud:"req email uniq"`
    CreatedAt  int64
    CreatedBy  uint64
    ModifiedAt int64
    ModifiedBy uint64
}

// User_Draft is the create payload — maps to the "user" table.
type User_Draft struct {
    ID    uint64
    Name  string `crud:"req len:1,100"`
    Email string `crud:"req email uniq"`
}

type Note struct {
    ID         uint64
    Title      string `crud:"req len:1,200"`
    Content    string
    Comment    string
    UserID     uint64 `crud:"req"`
    CreatedAt  int64
    CreatedBy  uint64
    ModifiedAt int64
    ModifiedBy uint64
}

// Note_Draft is the create payload — maps to the "note" table.
type Note_Draft struct {
    ID      uint64
    Title   string `crud:"req len:1,200"`
    Content string
    Comment string
    UserID  uint64 `crud:"req"`
}
```

The `_Draft` types intentionally omit audit fields (`CreatedAt`, `ModifiedAt`, etc.) so clients cannot set them directly.

## Database and service

```go
db, _ := sql.Open("sqlite", dbPath)

svc := svccrud.New(map[string]func() interface{}{
    "users": func() interface{} { return &User{} },
    "notes": func() interface{} { return &Note{} },
}, db, gocrud.DialectSQLite)

svc.CreateTables(context.Background())
```

`CreateTables` creates both tables if they do not already exist. The service registry maps URL path segments to their constructors.

## Hooks

Two hooks shape note behavior.

**`noteComment`** stamps every create and update with a server-controlled `Comment` field that clients cannot override:

```go
noteComment := func(obj interface{}, _ *http.Request) error {
    switch n := obj.(type) {
    case *Note:
        n.Comment = "Added with API"
    case *Note_Draft:
        n.Comment = "Added with API"
    }
    return nil
}
```

**`noteOwner`** rejects update and delete requests from users whose `X-User-ID` header does not match the note's stored `UserID`:

```go
noteOwner := func(obj interface{}, r *http.Request) error {
    note := obj.(*Note)
    headerUserID, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
    if note.UserID != headerUserID {
        return errors.New("not the note owner")
    }
    return nil
}
```

`AllowUpdate` receives the **stored** record before the request body is applied, so `noteOwner` always checks the original owner rather than a value the client could forge.

## Handler

```go
handler := gorestapi.New(svc, gorestapi.Options{
    UserIDFunc: func(r *http.Request) uint64 {
        id, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
        return id
    },
    Routes: map[string]gorestapi.Route{
        "users": {
            CreateConstructor: func() interface{} { return &User_Draft{} },
        },
        "notes": {
            CreateConstructor: func() interface{} { return &Note_Draft{} },
            AllowedFilters:    []string{"UserID"},
            AllowUpdate:       noteOwner,
            AllowDelete:       noteOwner,
            PreCreate:         noteComment,
            PreUpdate:         noteComment,
            PostRead: func(obj interface{}, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gorestapi"
                return nil
            },
            PostListItem: func(obj interface{}, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gorestapi"
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
```

`FilterList` and `FilterRead` automatically scope every notes query to the caller's user ID, so a user can only list and read their own notes. Injected filters take precedence over any client-supplied `filter_val_*` parameters.

## Mounting

```go
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    fmt.Fprintln(w, "ok")
})

http.ListenAndServe(":"+port, mux)
```

## Running

```
DB_PATH=poc.db PORT=8080 go run ./cmd/poc
```

| Env var | Default | Description |
|---|---|---|
| `DB_PATH` | `poc.db` | Path to the SQLite database file |
| `PORT` | `8080` | Port the server listens on |

## Trying it out

```bash
# Create a user
curl -s -X PUT http://localhost:8080/api/users/ \
  -H 'Content-Type: application/json' \
  -d '{"Name":"Alice","Email":"alice@example.com"}' | jq

# Create a note owned by user 1
curl -s -X PUT http://localhost:8080/api/notes/ \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 1' \
  -d '{"Title":"Hello","Content":"World","UserID":1}' | jq

# List notes — only the caller's notes are returned
curl -s http://localhost:8080/api/notes/ \
  -H 'X-User-ID: 1' | jq

# Read a single note (404 if UserID doesn't match)
curl -s http://localhost:8080/api/notes/1 \
  -H 'X-User-ID: 1' | jq

# Update a note (403 if not the owner)
curl -s -X PUT http://localhost:8080/api/notes/1 \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 1' \
  -d '{"Title":"Updated"}' | jq

# Delete a note (403 if not the owner)
curl -s -X DELETE http://localhost:8080/api/notes/1 \
  -H 'X-User-ID: 1' | jq
```

| Request | Behaviour |
|---|---|
| `GET /api/notes/` | Returns only the caller's notes (UserID injected from header) |
| `GET /api/notes/1` | Returns 404 if the note's UserID does not match the header |
| `PUT /api/notes/` | Creates a note; `Comment` is stamped by `PreCreate` |
| `PUT /api/notes/1` | Updates a note; returns 403 if the caller is not the owner |
| `DELETE /api/notes/1` | Deletes a note; returns 403 if the caller is not the owner |
