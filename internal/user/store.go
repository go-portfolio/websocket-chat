package user

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq" // Postgres driver
)

// Credentials — структура для логина/регистрации
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Avatar   string `json:"avatar"`
}

// UserStore — интерфейс для работы с пользователями
type UserStore interface {
	io.Closer
	Register(username, password, avatar string) error
	Authenticate(username, password string) bool
	GetAvatar(username string) string
}

// Store — реальная реализация UserStore для PostgreSQL
type Store struct {
	db *sql.DB
}

var _ UserStore = (*Store)(nil) // compile-time check

func NewStore(connStr string) (*Store, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(24) UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		avatar TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Register(username, password, avatar string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}
	if len(username) > 24 {
		return fmt.Errorf("username too long (max 24)")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	var avatarValue sql.NullString
	if avatar == "" {
		avatarValue = sql.NullString{Valid: false}
	} else {
		avatarValue = sql.NullString{String: avatar, Valid: true}
	}

	query := `INSERT INTO users (username, password_hash, created_at, avatar) VALUES ($1, $2, $3, $4)`
	_, err = s.db.Exec(query, username, string(hash), time.Now(), avatarValue)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return fmt.Errorf("username already exists")
		}
		return fmt.Errorf("failed to insert user: %w", err)
	}

	return nil
}

func (s *Store) Authenticate(username, password string) bool {
	var hash string
	query := `SELECT password_hash FROM users WHERE username=$1`
	err := s.db.QueryRow(query, username).Scan(&hash)
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Store) GetAvatar(username string) string {
	row := s.db.QueryRow(`SELECT avatar FROM users WHERE username=$1`, username)
	var avatar string
	_ = row.Scan(&avatar)
	return avatar
}
