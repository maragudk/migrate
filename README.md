# Migrate

[![GoDoc](https://godoc.org/github.com/maragudk/migrate?status.svg)](https://godoc.org/github.com/maragudk/migrate)
[![CircleCI](https://circleci.com/gh/maragudk/migrate.svg?style=shield)](https://circleci.com/gh/maragudk/migrate)

A simple database migration tool using an `sql.DB` connection and `fs.FS` for the migration source. It has no non-test dependencies.

This project is work-in-progress and has a lot of rough edges.

## Usage

```shell
go get -u github.com/maragudk/migrate
```

```go
package main

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/maragudk/migrate"
)

// migrations is a directory with sql files that look like this:
// migrations/1.up.sql
// migrations/1.down.sql
// migrations/2.up.sql
// migrations/2.down.sql
//go:embed migrations
var dir embed.FS

func main() {
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		panic(err)
	}
	migrations, err := fs.Sub(dir, "migrations")
	if err != nil {
		panic(err)
	}
	m := migrate.New(db, migrations)
	if err := m.MigrateUp(context.Background()); err != nil {
		panic(err)
	}

	if err := m.MigrateDown(context.Background()); err != nil {
		panic(err)
	}
}
```
