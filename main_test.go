package gorestapi

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/mikogs/gocrud"
	"github.com/mikogs/gocrud/pkg/gocrudtest"
	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/internal/fixtures"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerContainer *gocrudtest.Container

var testCRUD *gocrud.CRUD
var testService *registry.CRUD
var testHandler *Handler

func TestMain(m *testing.M) {
	dockerContainer, testDB = gocrudtest.CreateDocker(gocrudtest.PostgresDriver(), testUser, testPassword, testName)

	createCRUD()
	createService()
	createHandler()
	code := m.Run()

	_ = testDB.Close()
	gocrudtest.RemoveDocker(dockerContainer)
	os.Exit(gocrudtest.CheckLeaks(code))
}

func createCRUD() {
	testCRUD = gocrud.New(testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func createService() {
	testService = registry.New(map[string]func() any{
		"teststruct": func() any {
			return &fixtures.TestStruct{}
		},
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func createHandler() {
	testHandler = New(testService, Options{
		Routes: map[string]Route{
			"teststruct": {},
		},
	})
}

func recreateTestStructTable() {
	_ = testCRUD.DropTable(context.Background(), &fixtures.TestStruct{})
	_ = testCRUD.CreateTable(context.Background(), &fixtures.TestStruct{})
}
