package api

import (
	"context"
	"os"
	"testing"

	"github.com/harshalvk/cage/internal/store"
	"github.com/harshalvk/cage/internal/testutil"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var sharedContainer *tcpostgres.PostgresContainer
var sharedConnStr string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("cage_test"),
		tcpostgres.WithUsername("cage"),
		tcpostgres.WithPassword("cage"),
	)
	if err != nil {
		panic(err)
	}
	sharedContainer = container

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	sharedConnStr = connStr

	if err := testutil.RunMigrations(connStr); err != nil {
		panic(err)
	}

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func setupTestStore(t *testing.T) *store.Store {
	st, err := store.NewStore(context.Background(), sharedConnStr)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
