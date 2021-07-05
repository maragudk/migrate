package migrate_test

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/matryer/is"
	_ "github.com/mattn/go-sqlite3"

	"github.com/maragudk/migrate"
)

var testdata = os.DirFS("testdata")

func TestMigrator(t *testing.T) {
	tests := []struct {
		flavor         string
		createDatabase func(*testing.T) (*sql.DB, func())
	}{
		{"postgres", createPostgresDatabase},
		{"maria", createMariaDatabase},
		{"sqlite", createSQLiteDatabase},
	}

	for _, test := range tests {
		t.Run(test.flavor, func(t *testing.T) {
			t.Run("creates the migrations table if it does not exist", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, fstest.MapFS{})
				is.NoErr(err)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("", version)
			})

			t.Run("runs migrations up", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(2, count)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("3", version)
			})

			t.Run("does not error on another up", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)
			})

			t.Run("runs until a bad migration file", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "bad"))
				is.True(err != nil)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("1", version)
			})

			t.Run("runs migrations down", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.Down(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test limit 1`).Scan(&count)
				is.True(err != nil)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("", version)
			})

			t.Run("does not run down on newer migrations than current version", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Down(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)
			})

			t.Run("migrates up to version", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(1, count)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("2", version)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "3")
				is.NoErr(err)

				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(2, count)

				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("3", version)
			})

			t.Run("migrates down to version", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(1, count)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("2", version)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "1")
				is.NoErr(err)

				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(0, count)

				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("1", version)
			})

			t.Run("migrates to empty version", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.True(err != nil)

				var version string
				err = db.QueryRow(`select version from migrations`).Scan(&version)
				is.NoErr(err)
				is.Equal("", version)
			})

			t.Run("migrates to same version without error", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)
			})

			t.Run("migrate to errors if version not found", func(t *testing.T) {
				db, cleanup := test.createDatabase(t)
				defer cleanup()
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "doesnotexist")
				is.True(err != nil)
				is.Equal("error finding version doesnotexist", err.Error())
			})
		})
	}
}

var migrations = os.DirFS("testdata/example")

func Example() {
	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		panic(err)
	}

	if err := migrate.Up(context.Background(), db, migrations); err != nil {
		panic(err)
	}

	if err := migrate.Down(context.Background(), db, migrations); err != nil {
		panic(err)
	}

	if err := migrate.To(context.Background(), db, migrations, "1"); err != nil {
		panic(err)
	}
}

//go:embed testdata/example
var embeddedMigrations embed.FS

func Example_embed() {
	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		panic(err)
	}

	// Because migrate always reads from the root of the provided file system,
	// use fs.Sub to return the subtree rooted at the provided dir.
	fsys, err := fs.Sub(embeddedMigrations, "testdata/example")
	if err != nil {
		panic(err)
	}

	if err := migrate.Up(context.Background(), db, fsys); err != nil {
		panic(err)
	}

	if err := migrate.Down(context.Background(), db, fsys); err != nil {
		panic(err)
	}

	if err := migrate.To(context.Background(), db, fsys, "1"); err != nil {
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

func createMariaDatabase(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, err := sql.Open("mysql", "maria:123@/maria")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	return db, func() {
		if _, err := db.Exec(`drop table if exists migrations`); err != nil {
			t.Log(err)
			t.FailNow()
		}
		if _, err := db.Exec(`drop table if exists test`); err != nil {
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
