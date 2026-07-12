package gorestapi

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/mikolajgasior/gocrud"
	"github.com/mikolajgasior/gocrud/pkg/service"
	"github.com/mikolajgasior/gocrud/pkg/test"
	"github.com/ory/dockertest/v3"
)

var testUser = "testuser"
var testPassword = "testpass"
var testName = "testdb"
var testDB *sql.DB

var dockerPool *dockertest.Pool
var dockerResource *dockertest.Resource

var testCRUD *gocrud.CRUD
var testService *service.CRUD
var testHandler *Handler

func TestMain(m *testing.M) {
	dockerPool, dockerResource, testDB = test.CreateDocker(testUser, testPassword, testName)

	var code int

	defer func() {
		test.RemoveDocker(dockerPool, dockerResource)
		os.Exit(code)
	}()

	createCRUD()
	createService()
	createHandler()

	code = m.Run()
}

func createCRUD() {
	testCRUD = gocrud.New(testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})
}

func createService() {
	testService = service.New(map[string]func() interface{}{
		"teststruct": func() interface{} {
			return &test.TestStruct{}
		},
	}, testDB, gocrud.DialectPostgres)
}

func createHandler() {
	testHandler = New(testService, Options{
		Routes: map[string]Route{
			"teststruct": {},
		},
	})
}

func recreateTestStructTable() {
	_ = testCRUD.DropTable(context.Background(), &test.TestStruct{})
	_ = testCRUD.CreateTable(context.Background(), &test.TestStruct{})
}
