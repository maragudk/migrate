// Package migrate provides simple migration functions Up, Down, and To, as well as a Migrator.
// Up, Down, and To are one-liner convenience functions that use default Options.
// If you need custom Options, use New.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
)

var (
	upMatcher    = regexp.MustCompile(`^([\w-]+).up.sql$`)
	downMatcher  = regexp.MustCompile(`^([\w-]+).down.sql`)
	tableMatcher = regexp.MustCompile(`^[\w.]+$`)
)

// Up from the current version.
func Up(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	m := New(Options{DB: db, FS: fsys})
	return m.MigrateUp(ctx)
}

// Down from the current version.
func Down(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	m := New(Options{DB: db, FS: fsys})
	return m.MigrateDown(ctx)
}

// To the given version.
func To(ctx context.Context, db *sql.DB, fsys fs.FS, version string) error {
	m := New(Options{DB: db, FS: fsys})
	return m.MigrateTo(ctx, version)
}

// callback that can be run before and after each migration.
type callback = func(ctx context.Context, tx *sql.Tx, version string) error

type Migrator struct {
	after  callback
	before callback
	db     *sql.DB
	fs     fs.FS
	table  string
}

// Options for New. DB and FS are always required.
type Options struct {
	After  callback
	Before callback
	DB     *sql.DB
	FS     fs.FS
	Table  string
}

// New Migrator with Options.
// If Options.Table is not set, defaults to "migrations". The table name must match ^[\w.]+$ .
// New panics on illegal options.
func New(opts Options) *Migrator {
	if opts.DB == nil || opts.FS == nil {
		panic("DB and FS must be set")
	}
	if opts.Table == "" {
		opts.Table = "migrations"
	}
	if !tableMatcher.MatchString(opts.Table) {
		panic("illegal table name " + opts.Table + ", must match " + tableMatcher.String())
	}
	return &Migrator{
		after:  opts.After,
		before: opts.Before,
		db:     opts.DB,
		fs:     opts.FS,
		table:  opts.Table,
	}
}

// MigrateUp from the current version.
func (m *Migrator) MigrateUp(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("error migrating up: %w", err)
		}
	}()

	if err := m.createMigrationsTable(ctx); err != nil {
		return err
	}

	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		return err
	}

	names, err := m.getFilenames(upMatcher)
	if err != nil {
		return err
	}

	for _, name := range names {
		thisVersion := upMatcher.ReplaceAllString(name, "$1")
		if thisVersion <= currentVersion {
			continue
		}

		if err := m.apply(ctx, name, thisVersion); err != nil {
			return err
		}
	}

	return nil
}

// MigrateDown from the current version.
func (m *Migrator) MigrateDown(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("error migrating down: %w", err)
		}
	}()

	if err := m.createMigrationsTable(ctx); err != nil {
		return err
	}

	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		return err
	}

	names, err := m.getFilenames(downMatcher)
	if err != nil {
		return err
	}

	for i := len(names) - 1; i >= 0; i-- {
		thisVersion := downMatcher.ReplaceAllString(names[i], "$1")
		if thisVersion > currentVersion {
			continue
		}

		nextVersion := ""
		if i > 0 {
			nextVersion = downMatcher.ReplaceAllString(names[i-1], "$1")
		}

		if err := m.apply(ctx, names[i], nextVersion); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) MigrateTo(ctx context.Context, version string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("error migrating to: %w", err)
		}
	}()

	if version == "" {
		return m.MigrateDown(ctx)
	}

	if err := m.createMigrationsTable(ctx); err != nil {
		return err
	}

	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		return err
	}

	if currentVersion == version {
		return nil
	}

	var matcher *regexp.Regexp
	if version > currentVersion {
		matcher = upMatcher
	} else {
		matcher = downMatcher
	}
	names, err := m.getFilenames(matcher)
	if err != nil {
		return err
	}

	foundVersion := false
	for _, name := range names {
		thisVersion := matcher.ReplaceAllString(name, "$1")
		if thisVersion == version {
			foundVersion = true
		}
	}
	if !foundVersion {
		return errors.New("error finding version " + version)
	}

	switch {
	case version > currentVersion:
		for _, name := range names {
			thisVersion := matcher.ReplaceAllString(name, "$1")
			if thisVersion <= currentVersion {
				continue
			}
			if thisVersion > version {
				break
			}

			if err := m.apply(ctx, name, thisVersion); err != nil {
				return err
			}
		}
	case version < currentVersion:
		for i := len(names) - 1; i >= 0; i-- {
			thisVersion := matcher.ReplaceAllString(names[i], "$1")
			if thisVersion > currentVersion {
				continue
			}

			if thisVersion <= version {
				break
			}

			nextVersion := matcher.ReplaceAllString(names[i-1], "$1")

			if err := m.apply(ctx, names[i], nextVersion); err != nil {
				return err
			}
		}
	}

	return nil
}

// apply a file identified by name and update to version.
func (m *Migrator) apply(ctx context.Context, name, version string) error {
	content, err := fs.ReadFile(m.fs, name)
	if err != nil {
		return fmt.Errorf("error reading migration file %v: %w", name, err)
	}

	return m.inTransaction(ctx, func(tx *sql.Tx) error {
		if m.before != nil {
			if err := m.before(ctx, tx, version); err != nil {
				return fmt.Errorf("error in 'before' callback when applying version %v from %v: %w", version, name, err)
			}
		}

		// Normally we wouldn't just string interpolate the version like this,
		// but because we know the version has been matched against the regexes, we know it's safe.
		if _, err := tx.ExecContext(ctx, `update `+m.table+` set version = '`+version+`'`); err != nil {
			return fmt.Errorf("error updating version to %v: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("error running migration %v from %v: %w", version, name, err)
		}

		if m.after != nil {
			if err := m.after(ctx, tx, version); err != nil {
				return fmt.Errorf("error in 'after' callback when applying version %v from %v: %w", version, name, err)
			}
		}
		return nil
	})
}

// getFilenames alphabetically where the name matches the given matcher.
func (m *Migrator) getFilenames(matcher *regexp.Regexp) ([]string, error) {
	var names []string
	entries, err := fs.ReadDir(m.fs, ".")
	if err != nil {
		return names, err
	}

	for _, entry := range entries {
		if !matcher.MatchString(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// createMigrationsTable if it does not exist already, and insert the empty version if it's empty.
func (m *Migrator) createMigrationsTable(ctx context.Context) error {
	return m.inTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `create table if not exists `+m.table+` (version text not null)`); err != nil {
			return fmt.Errorf("error creating migrations table %v: %w", m.table, err)
		}

		var exists bool
		if err := tx.QueryRowContext(ctx, `select exists (select * from `+m.table+`)`).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			if _, err := tx.ExecContext(ctx, `insert into `+m.table+` values ('')`); err != nil {
				return err
			}
		}
		return nil
	})
}

// getCurrentVersion from the migrations table.
func (m *Migrator) getCurrentVersion(ctx context.Context) (string, error) {
	var version string
	if err := m.db.QueryRowContext(ctx, `select version from `+m.table+``).Scan(&version); err != nil {
		return "", fmt.Errorf("error getting current migration version: %w", err)
	}
	return version, nil
}

func (m *Migrator) inTransaction(ctx context.Context, callback func(tx *sql.Tx) error) (err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			err = rollback(tx, fmt.Errorf("panic: %v", rec))
		}
	}()
	if err := callback(tx); err != nil {
		return rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func rollback(tx *sql.Tx, err error) error {
	if txErr := tx.Rollback(); txErr != nil {
		return fmt.Errorf("error rolling back transaction after error (transaction error: %v), original error: %w", txErr, err)
	}
	return err
}
