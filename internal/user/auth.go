package user

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

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
