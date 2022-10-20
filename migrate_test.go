package migrate_test

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"os"
	"strings"
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
		createDatabase func(*testing.T) *sql.DB
	}{
		{"postgres", createPostgresDatabase},
		{"maria", createMariaDatabase},
		{"sqlite", createSQLiteDatabase},
	}

	for _, test := range tests {
		t.Run(test.flavor, func(t *testing.T) {
			t.Run("creates the migrations table if it does not exist", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, fstest.MapFS{})
				is.NoErr(err)

				version := getVersion(t, db)
				is.Equal("", version)
			})

			t.Run("runs migrations up", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(2, count)

				version := getVersion(t, db)
				is.Equal("3", version)
			})

			t.Run("does not error on another up", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)
			})

			t.Run("runs until a bad migration file", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "bad"))
				is.True(err != nil)
				is.True(strings.Contains(err.Error(), "error migrating up: error running migration 2 from 2.up.sql"))

				version := getVersion(t, db)
				is.Equal("1", version)
			})

			t.Run("runs migrations down", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.Down(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.True(err != nil)

				version := getVersion(t, db)
				is.Equal("", version)
			})

			t.Run("does not run down on newer migrations than current version", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Down(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)
			})

			t.Run("migrates up to version", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(1, count)

				version := getVersion(t, db)
				is.Equal("2", version)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "3")
				is.NoErr(err)

				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(2, count)

				version = getVersion(t, db)
				is.Equal("3", version)
			})

			t.Run("migrates down to version", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(1, count)

				version := getVersion(t, db)
				is.Equal("2", version)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "1")
				is.NoErr(err)

				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.NoErr(err)
				is.Equal(0, count)

				version = getVersion(t, db)
				is.Equal("1", version)
			})

			t.Run("migrates to empty version", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.Up(context.Background(), db, mustSub(t, testdata, "good"))
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "")
				is.NoErr(err)

				var count int
				err = db.QueryRow(`select count(*) from test`).Scan(&count)
				is.True(err != nil)

				version := getVersion(t, db)
				is.Equal("", version)
			})

			t.Run("migrates to same version without error", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)

				err = migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "2")
				is.NoErr(err)
			})

			t.Run("migrate to errors if version not found", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				err := migrate.To(context.Background(), db, mustSub(t, testdata, "good"), "doesnotexist")
				is.True(err != nil)
				is.Equal("error migrating to: error finding version doesnotexist", err.Error())
			})

			t.Run("supports custom table name", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				m := migrate.New(migrate.Options{DB: db, FS: mustSub(t, testdata, "good"), Table: "migrations2"})
				err := m.MigrateUp(context.Background())
				is.NoErr(err)

				var version string
				err = db.QueryRow(`select version from migrations2`).Scan(&version)
				is.NoErr(err)
				is.Equal("3", version)
			})

			t.Run("can run callbacks before and after each migration", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				var beforeCalled, afterCalled bool
				before := func(ctx context.Context, tx *sql.Tx, version string) error {
					beforeCalled = true
					is.Equal(version, "1")
					return nil
				}

				after := func(ctx context.Context, tx *sql.Tx, version string) error {
					afterCalled = true
					is.Equal(version, "1")
					return nil
				}

				m := migrate.New(migrate.Options{DB: db, FS: mustSub(t, testdata, "good"), Before: before, After: after})
				err := m.MigrateTo(context.Background(), "1")
				is.NoErr(err)
				is.True(beforeCalled)
				is.True(afterCalled)
			})

			t.Run("aborts migration if before callback fails", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				before := func(ctx context.Context, tx *sql.Tx, version string) error {
					return errors.New("oh no")
				}

				m := migrate.New(migrate.Options{DB: db, FS: mustSub(t, testdata, "good"), Before: before})
				err := m.MigrateUp(context.Background())
				is.True(err != nil)
				is.True(strings.Contains(err.Error(), "error migrating up: error in 'before' callback when applying version 1 from 1.up.sql: oh no"))

				version := getVersion(t, db)
				is.Equal("", version)
			})

			t.Run("aborts migration if after callback fails", func(t *testing.T) {
				db := test.createDatabase(t)
				is := is.New(t)

				// We migrate to version 1 first, because not all databases support DDL changes inside transactions
				// (or maybe implicitly commit the transaction if they occur).
				fsys := mustSub(t, testdata, "good")
				err := migrate.To(context.Background(), db, fsys, "1")
				is.NoErr(err)

				after := func(ctx context.Context, tx *sql.Tx, version string) error {
					return errors.New("oh no")
				}

				m := migrate.New(migrate.Options{DB: db, FS: fsys, After: after})
				err = m.MigrateUp(context.Background())
				is.True(err != nil)
				is.True(strings.Contains(err.Error(), "error migrating up: error in 'after' callback when applying version 2 from 2.up.sql: oh no"))

				version := getVersion(t, db)
				is.Equal("1", version)
			})
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("panics on bad table name", func(t *testing.T) {
		is := is.New(t)

		defer func() {
			err := recover()
			is.True(err != nil)
			is.Equal(`illegal table name +, must match ^[\w.]+$`, err)
		}()
		migrate.New(migrate.Options{DB: &sql.DB{}, FS: fstest.MapFS{}, Table: "+"})
	})

	t.Run("support table name containing dot", func(t *testing.T) {
		is := is.New(t)

		defer func() {
			err := recover()
			is.True(err == nil)
		}()
		migrate.New(migrate.Options{DB: &sql.DB{}, FS: fstest.MapFS{}, Table: "schema.mytable"})
	})

	t.Run("panics on no db given", func(t *testing.T) {
		is := is.New(t)

		defer func() {
			err := recover()
			is.True(err != nil)
			is.Equal(`DB and FS must be set`, err)
		}()
		migrate.New(migrate.Options{FS: fstest.MapFS{}})
	})

	t.Run("panics on no fs given", func(t *testing.T) {
		is := is.New(t)

		defer func() {
			err := recover()
			is.True(err != nil)
			is.Equal(`DB and FS must be set`, err)
		}()
		migrate.New(migrate.Options{DB: &sql.DB{}})
	})
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

func Example_advanced() {
	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		panic(err)
	}

	before := func(ctx context.Context, tx *sql.Tx, version string) error {
		// Do whatever you need to before each migration
		return nil
	}

	after := func(ctx context.Context, tx *sql.Tx, version string) error {
		// Do whatever you need to after each migration
		return nil
	}

	m := migrate.New(migrate.Options{
		After:  after,
		Before: before,
		DB:     db,
		FS:     migrations,
		Table:  "migrations2",
	})

	if err := m.MigrateUp(context.Background()); err != nil {
		panic(err)
	}

	if err := m.MigrateDown(context.Background()); err != nil {
		panic(err)
	}

	if err := m.MigrateTo(context.Background(), "1"); err != nil {
		panic(err)
	}
}

func createPostgresDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", "postgresql://postgres:123@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`drop table if exists migrations; drop table if exists migrations2; drop table if exists test`); err != nil {
			t.Fatal(err)
		}
	})
	return db
}

func createSQLiteDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Log(err)
		}
		if err := os.Remove("db.sqlite"); err != nil {
			t.Fatal(err)
		}
	})
	return db
}

func createMariaDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "maria:123@/maria")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`drop table if exists migrations`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`drop table if exists migrations2`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`drop table if exists test`); err != nil {
			t.Fatal(err)
		}
	})
	return db
}

func mustSub(t *testing.T, fsys fs.FS, path string) fs.FS {
	t.Helper()
	fsys, err := fs.Sub(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}

func getVersion(t *testing.T, db *sql.DB) string {
	t.Helper()
	var version string
	err := db.QueryRow(`select version from migrations`).Scan(&version)
	if err != nil {
		t.Fatal(err)
	}
	return version
}
