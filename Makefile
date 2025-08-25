SHELL := /bin/bash
ENV_FILE := .env

DATABASE_URL := $(shell grep -v '^#' $(ENV_FILE) | grep DATABASE_URL | cut -d '=' -f2-)

.PHONY: migrate-up migrate-down

migrate-up:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate \
		-path ./migrations -database "$(DATABASE_URL)" up

migrate-down:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate \
		-path ./migrations -database "$(DATABASE_URL)" down
