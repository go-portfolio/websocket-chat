-- +migrate Up
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(24) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    avatar VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
