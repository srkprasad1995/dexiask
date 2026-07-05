// Package database wires the GORM-managed PostgreSQL connection and migrations.
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
)

// NewPostgresDB opens a GORM-managed PostgreSQL connection pool from a DSN. The
// DSN may be either URL form (postgres://user:pass@host:port/db?sslmode=disable)
// or key=value form — the pgx driver accepts both.
func NewPostgresDB(dsn string, log *logger.Logger) (*gorm.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DEXIASK_DB_DSN is required")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	log.Info("database connected successfully")
	return db, nil
}

// MigrateDB runs schema migrations. Safe to call on every startup.
func MigrateDB(db *gorm.DB, log *logger.Logger) error {
	log.Info("running database migrations")

	if err := db.AutoMigrate(
		&model.User{},
		&model.Session{},
		&model.Invite{},
		&model.Conversation{},
		&model.Message{},
		&model.Attachment{},
		&model.SlackThread{},
		&model.MCPServer{},
	); err != nil {
		return fmt.Errorf("failed to run AutoMigrate: %w", err)
	}

	// Any message left 'running' from a previous crash will never complete —
	// mark them 'partial' so the assembler and UI handle them gracefully. Safe
	// because RunManager starts empty on every restart.
	if err := db.Exec("UPDATE messages SET status = 'partial' WHERE status = 'running'").Error; err != nil {
		return fmt.Errorf("failed to sweep running messages: %w", err)
	}

	log.Info("database migrations completed successfully")
	return nil
}

// CloseDB closes the connection pool.
func CloseDB(db *gorm.DB, log *logger.Logger) error {
	log.Info("closing database connection")
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// HealthCheck pings the database with a short timeout.
func HealthCheck(db *gorm.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
