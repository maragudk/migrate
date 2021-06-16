package migrate_test

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"

	"github.com/maragudk/migrate"
)

//go:embed testdata
var testdata embed.FS

func TestMigrator(t *testing.T) {
	tt := []struct {
		driver         string
		createDatabase func(*testing.T) (*sql.DB, func())
	}{
		{"postgres/", createPostgresDatabase},
		{"sqlite/", createSQLiteDatabase},
	}

	for _, test := range tt {
		t.Run(test.driver+"creates the migrations table if it does not exist", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: fstest.MapFS{},
			}

			err := m.MigrateUp(context.Background())
			is.NoErr(err)

			var version string
			err = db.QueryRow(`select version from migrations`).Scan(&version)
			is.NoErr(err)
			is.Equal("", version)
		})

		t.Run(test.driver+"runs migrations up", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: mustSub(t, testdata, "testdata/two"),
			}

			err := m.MigrateUp(context.Background())
			is.NoErr(err)

			var count int
			err = db.QueryRow(`select count(*) from test`).Scan(&count)
			is.NoErr(err)
			is.Equal(2, count)
		})

		t.Run(test.driver+"does not error on another up", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: mustSub(t, testdata, "testdata/two"),
			}

			err := m.MigrateUp(context.Background())
			is.NoErr(err)

			err = m.MigrateUp(context.Background())
			is.NoErr(err)
		})

		t.Run(test.driver+"runs until a bad migration file", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: mustSub(t, testdata, "testdata/bad"),
			}

			err := m.MigrateUp(context.Background())
			is.True(err != nil)

			var version string
			err = db.QueryRow(`select version from migrations`).Scan(&version)
			is.NoErr(err)
			is.Equal("1", version)
		})

		t.Run(test.driver+"runs migrations down", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: mustSub(t, testdata, "testdata/two"),
			}

			err := m.MigrateUp(context.Background())
			is.NoErr(err)

			err = m.MigrateDown(context.Background())
			is.NoErr(err)

			var count int
			err = db.QueryRow(`select count(*) from test limit 1`).Scan(&count)
			is.True(err != nil)

			var version string
			err = db.QueryRow(`select version from migrations`).Scan(&version)
			is.NoErr(err)
			is.Equal("", version)
		})

		t.Run(test.driver+"does not run down on newer migrations than current version", func(t *testing.T) {
			db, cleanup := test.createDatabase(t)
			defer cleanup()
			is := is.New(t)

			m := migrate.Migrator{
				DB: db,
				FS: mustSub(t, testdata, "testdata/two"),
			}

			err := m.MigrateDown(context.Background())
			is.NoErr(err)
		})
	}
}

//go:embed testdata/example
var exampleFS embed.FS

func Example() {
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		panic(err)
	}
	migrations, err := fs.Sub(exampleFS, "testdata/example")
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

func createPostgresDatabase(t *testing.T) (*sql.DB, func()) {
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

func createSQLiteDatabase(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	return db, func() {
		if err := db.Close(); err != nil {
			t.Log(err)
		}
		if err := os.Remove("db.sqlite"); err != nil {
			t.Log(err)
			t.FailNow()
		}
	}
}

func mustSub(t *testing.T, fsys fs.FS, path string) fs.FS {
	t.Helper()
	fsys, err := fs.Sub(fsys, path)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	return fsys
}
