package migrate_test

import (
	"context"
	"database/sql"
	"embed"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/matryer/is"

	"github.com/maragudk/migrate"
)

//go:embed testdata
var testdata embed.FS

func TestMigrator_MigrateUp(t *testing.T) {
	t.Run("creates the migrations table if it does not exist", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   fstest.MapFS{},
			Path: ".",
		}

		err := m.MigrateUp(context.Background())
		is.NoErr(err)

		var version string
		err = db.QueryRow(`select version from migrations`).Scan(&version)
		is.NoErr(err)
		is.Equal("", version)
	})

	t.Run("runs migrations up", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   testdata,
			Path: "testdata/two",
		}

		err := m.MigrateUp(context.Background())
		is.NoErr(err)

		var count int
		err = db.QueryRow(`select count(*) from test`).Scan(&count)
		is.NoErr(err)
		is.Equal(2, count)
	})

	t.Run("does not error on another up", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   testdata,
			Path: "testdata/two",
		}

		err := m.MigrateUp(context.Background())
		is.NoErr(err)

		err = m.MigrateUp(context.Background())
		is.NoErr(err)
	})

	t.Run("runs until a bad migration file", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   testdata,
			Path: "testdata/bad",
		}

		err := m.MigrateUp(context.Background())
		is.True(err != nil)
		is.Equal(`ERROR: relation "doesnotexist" does not exist (SQLSTATE 42P01)`, err.Error())

		var version string
		err = db.QueryRow(`select version from migrations`).Scan(&version)
		is.NoErr(err)
		is.Equal("1", version)
	})
}

func TestMigrator_MigrateDown(t *testing.T) {
	t.Run("runs migrations down", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   testdata,
			Path: "testdata/two",
		}

		err := m.MigrateUp(context.Background())
		is.NoErr(err)

		err = m.MigrateDown(context.Background())
		is.NoErr(err)

		var exists bool
		err = db.QueryRow(`select exists (select * from information_schema.tables where table_name = 'test')`).Scan(&exists)
		is.NoErr(err)
		is.True(!exists)

		var version string
		err = db.QueryRow(`select version from migrations`).Scan(&version)
		is.NoErr(err)
		is.Equal("", version)
	})

	t.Run("does not run down on newer migrations than current version", func(t *testing.T) {
		db, cleanup := createDatabase(t)
		defer cleanup()
		is := is.New(t)

		m := migrate.Migrator{
			DB:   db,
			FS:   testdata,
			Path: "testdata/two",
		}

		err := m.MigrateDown(context.Background())
		is.NoErr(err)
	})
}

//go:embed testdata/example
var exampleFS embed.FS

func Example() {
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		panic(err)
	}
	m := migrate.New(db, exampleFS)
	m.Path = "testdata/example"
	if err := m.MigrateUp(context.Background()); err != nil {
		panic(err)
	}

	if err := m.MigrateDown(context.Background()); err != nil {
		panic(err)
	}
}

func createDatabase(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	return db, func() {
		if _, err := db.Exec(`drop table if exists migrations; drop table if exists test`); err != nil {
			t.Log(err)
			t.FailNow()
		}
	}
}
