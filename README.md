# Migrate

[![GoDoc](https://pkg.go.dev/badge/maragu.dev/migrate)](https://pkg.go.dev/maragu.dev/migrate)
[![Go](https://github.com/maragudk/migrate/actions/workflows/ci.yml/badge.svg)](https://github.com/maragudk/migrate/actions/workflows/ci.yml)

A simple database migration tool using an `sql.DB` connection and `fs.FS` for the migration source. It has no non-test dependencies.

Made in 🇩🇰 by [maragu](https://www.maragu.dk), maker of [online Go courses](https://www.golang.dk/).

## Features

- Simple: The common usage is a one-liner.
- Safe: Each migration is run in a transaction, and automatically rolled back on errors.
- Flexible: Setup a custom migrations table and use callbacks before and after each migration.

## Usage

```shell
go get maragu.dev/migrate
```

```go
package main

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/jackc/pgx/v4/stdlib"
	"maragu.dev/migrate"
)

// migrations is a directory with sql files that look something like this:
// migrations/1-accounts.up.sql
// migrations/1-accounts.down.sql
// migrations/2-users.up.sql
// migrations/2-users.down.sql
var migrations = os.DirFS("migrations")

func main() {
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		panic(err)
	}

	if err := migrate.Up(context.Background(), db, migrations); err != nil {
		panic(err)
	}

	if err := migrate.Down(context.Background(), db, migrations); err != nil {
		panic(err)
	}

	if err := migrate.To(context.Background(), db, migrations, "1-accounts"); err != nil {
		panic(err)
	}
}
```

### Helper tool

To install the helper tool, run:

```shell
go install maragu.dev/migrate/cmd/migrate@latest
```

Then you can run `migrate` in your terminal.

```shell
migrate create sql/migrations accounts
```
