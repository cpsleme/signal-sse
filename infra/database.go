package infra

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// HistoryDB provides database operations for tb_history and tb_attachments.
type HistoryDB struct {
	DB *sql.DB
}

// ConnectHistoryDB initializes and returns a new HistoryDB instance for MySQL.
// It takes the MySQL DSN (Data Source Name) as input.
func ConnectHistoryDB(mysqlDSN string) (*HistoryDB, error) {
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to MySQL database: %w using %s", err, mysqlDSN)
	}

	log.Printf("Successfully connected to MySQL database using DSN.")
	return &HistoryDB{DB: db}, nil
}

// Close the database connection.
func (h *HistoryDB) Close() error {
	return h.DB.Close()
}
