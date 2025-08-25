package user

import (
	"database/sql"
	"fmt"
	"io"

	_ "github.com/lib/pq"
)

type UserStore interface {
	io.Closer
	Register(username, password, avatar string) error
	Authenticate(username, password string) bool
	GetAvatar(username string) string
}

type Store struct {
	db *sql.DB
}

var _ UserStore = (*Store)(nil)

func NewStore(connStr string) (*Store, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
