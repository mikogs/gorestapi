package jwtauth

import (
	"os"
	"testing"

	"github.com/mikogs/gocrud/pkg/gocrudtest"
)

func TestMain(m *testing.M) {
	os.Exit(gocrudtest.CheckLeaks(m.Run()))
}
