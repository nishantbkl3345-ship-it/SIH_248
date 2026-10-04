// Package database opens the PostgreSQL connection and runs migrations.
package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"fogline/api/migrations"
)

// Open connects to PostgreSQL and verifies the connection. Schema changes are
// made only through migrations; GORM AutoMigrate is deliberately not used.
func Open(ctx context.Context, url string, verbose bool) (*gorm.DB, error) {
	level := gormlogger.Silent
	if verbose {
		level = gormlogger.Warn
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: gormlogger.New(log.New(os.Stderr, "gorm: ", log.LstdFlags), gormlogger.Config{
			LogLevel:                  level,
			SlowThreshold:             500 * time.Millisecond,
			IgnoreRecordNotFoundError: true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return db, nil
}

// MigrateUp applies every pending migration.
func MigrateUp(url string) error {
	return withMigrator(url, func(m *migrate.Migrate) error { return m.Up() })
}

// MigrateDown rolls back every applied migration.
func MigrateDown(url string) error {
	return withMigrator(url, func(m *migrate.Migrate) error { return m.Down() })
}

func withMigrator(url string, run func(*migrate.Migrate) error) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("database: migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL(url))
	if err != nil {
		return fmt.Errorf("database: migrator: %w", err)
	}
	defer m.Close()

	if err := run(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: migrate: %w", err)
	}
	return nil
}

// migrateURL rewrites a postgres URL to the scheme golang-migrate's pgx v5
// driver is registered under.
func migrateURL(url string) string {
	for _, scheme := range []string{"postgresql://", "postgres://"} {
		if rest, ok := strings.CutPrefix(url, scheme); ok {
			return "pgx5://" + rest
		}
	}
	return url
}
