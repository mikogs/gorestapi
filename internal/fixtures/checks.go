package fixtures

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

const idField = "ID"

func objFieldInterfaces(obj any, includeID bool) []any {
	objValue := reflect.ValueOf(obj).Elem()

	var fieldInterfaces []any

	for i := 0; i < objValue.NumField(); i++ {
		valueField := objValue.Field(i)
		if objValue.Type().Field(i).Name == idField && !includeID {
			continue
		}

		if !objValue.Type().Field(i).IsExported() {
			continue
		}

		fieldInterfaces = append(fieldInterfaces, valueField.Addr().Interface())
	}

	return fieldInterfaces
}

func FatalIfTestStructNotDeletedInTheDatabase(t *testing.T, db *sql.DB, id uint64) {
	var rowCount uint64
	err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) AS c FROM test_struct WHERE id = $1", id).Scan(&rowCount)
	if err != nil {
		t.Fatalf("Delete failed to delete struct from the table")
	}
	if rowCount > 0 {
		t.Fatalf("Delete failed to delete struct from the table")
	}
}

func FatalIfNoTestStructInTheDatabase(t *testing.T, db *sql.DB) *TestStruct {
	objLoaded := &TestStruct{}
	fieldsPtrs := []any{&objLoaded.ID}
	fieldsPtrs = append(fieldsPtrs, objFieldInterfaces(objLoaded, false)...)
	err := db.QueryRowContext(context.Background(), "SELECT * FROM test_struct ORDER BY id DESC LIMIT 1").Scan(fieldsPtrs...)
	if err != nil {
		t.Fatalf("Save failed to insert struct to the table: %s", err.Error())
	}

	return objLoaded
}
