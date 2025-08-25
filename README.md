# Go WebSocket Чат

## Стартовать сервер чата
```go
go run cmd/server/main.go
```

# TODO расписать
как запустить локально (go run, curl примеры)

как собрать в Docker

demo-сценарий (регистрация → вход → чат)

Создание Баз Данных:
sudo -u postgres psql


CREATE DATABASE chatapp;
CREATE USER chatuser WITH PASSWORD 'secret';
GRANT ALL PRIVILEGES ON DATABASE chatapp TO chatuser;


