package testharness

import (
	"context"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"testing"
)

const (
	PostgresPassword = "123qwerty"
	PostgresUsername = "postgres"
	PostgresDatabase = "test"
)

func PreparePostgres(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()
	if testing.Short() {
		t.Skip("PostgreSQL integration tests require Docker")
	}
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16.9-alpine",
		postgres.WithDatabase(PostgresDatabase),
		postgres.WithUsername(PostgresUsername),
		postgres.WithPassword(PostgresPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL test container: %v", err)
	}

	return postgresContainer
}
