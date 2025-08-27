SHELL := /bin/bash
ENV_FILE := .env

DATABASE_URL := $(shell grep -v '^#' $(ENV_FILE) | grep DATABASE_URL | cut -d '=' -f2-)

.PHONY: migrate-up migrate-down

migrate-up:
	migrate -path ./migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path ./migrations -database "$(DATABASE_URL)" down
