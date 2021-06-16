// Package migrate provides a simple Migrator that can migrate databases.
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
	upMatcher   = regexp.MustCompile(`^([\w]+).up.sql$`)
	downMatcher = regexp.MustCompile(`^([\w]+).down.sql`)
)

type Migrator struct {
	DB *sql.DB
	FS fs.FS
}

// New Migrator with default options.
func New(db *sql.DB, fs fs.FS) *Migrator {
	return &Migrator{
		DB: db,
		FS: fs,
	}
}

// MigrateUp from the current version.
func (m *Migrator) MigrateUp(ctx context.Context) error {
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
func (m *Migrator) MigrateDown(ctx context.Context) error {
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
		name := names[i]
		thisVersion := downMatcher.ReplaceAllString(name, "$1")
		if thisVersion > currentVersion {
			continue
		}

		nextVersion := ""
		if i > 0 {
			nextVersion = downMatcher.ReplaceAllString(names[i-1], "$1")
		}

		if err := m.apply(ctx, name, nextVersion); err != nil {
			return err
		}
	}

	return nil
}

// apply a file identified by name and update to version.
func (m *Migrator) apply(ctx context.Context, name, version string) error {
	content, err := fs.ReadFile(m.FS, name)
	if err != nil {
		return err
	}
	return m.inTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `update migrations set version = $1`, version); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return err
		}
		return nil
	})
}

// getFilenames alphabetically where the name matches the given matcher.
func (m *Migrator) getFilenames(matcher *regexp.Regexp) ([]string, error) {
	var names []string
	entries, err := fs.ReadDir(m.FS, ".")
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
		if _, err := tx.ExecContext(ctx, `create table if not exists migrations (version text not null)`); err != nil {
			return err
		}

		var exists bool
		if err := tx.QueryRowContext(ctx, `select exists (select * from migrations)`).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			if _, err := tx.ExecContext(ctx, `insert into migrations values ('')`); err != nil {
				return err
			}
		}
		return nil
	})
}

// getCurrentVersion from the migrations table.
func (m *Migrator) getCurrentVersion(ctx context.Context) (string, error) {
	var version string
	if err := m.DB.QueryRowContext(ctx, `select version from migrations`).Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

func (m *Migrator) inTransaction(ctx context.Context, callback func(tx *sql.Tx) error) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return errors.New("error beginning transaction: " + err.Error())
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
		return errors.New("error committing transaction: " + err.Error())
	}

	return nil
}

func rollback(tx *sql.Tx, err error) error {
	if txErr := tx.Rollback(); txErr != nil {
		return fmt.Errorf("error rolling back transaction after error (transaction error: %v), original error: %v", txErr.Error(), err.Error())
	}
	return err
}
