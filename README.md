# Migrate

[![GoDoc](https://godoc.org/github.com/maragudk/migrate?status.svg)](https://godoc.org/github.com/maragudk/migrate)
[![CircleCI](https://circleci.com/gh/maragudk/migrate.svg?style=shield)](https://circleci.com/gh/maragudk/migrate)

A simple database migration tool using an `sql.DB` connection and `fs.FS` for the migration source. It has no non-test dependencies.

Made in 🇩🇰 by [maragu](https://www.maragu.dk), maker of [online Go courses](https://www.golang.dk/).

## Usage

```shell
go get -u github.com/maragudk/migrate
```

```go
package main

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/maragudk/migrate"
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
