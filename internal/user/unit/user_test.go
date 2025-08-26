package user_test

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock" // библиотека для моков SQL-запросов
	"github.com/go-portfolio/websocket-chat/internal/user"
	"github.com/stretchr/testify/assert" // удобные ассерты
	"golang.org/x/crypto/bcrypt"        // для генерации и проверки хэшей паролей
)

// --- ТЕСТЫ ДЛЯ Register ---
// Register — метод добавления нового пользователя в БД

func TestRegister_Success(t *testing.T) {
	// создаём фейковую (mock) базу и объект Store
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Ожидаем выполнение SQL-запроса INSERT с правильными аргументами.
	// regexp.QuoteMeta используется, чтобы экранировать спецсимволы в SQL.
	// sqlmock.AnyArg() означает "любое значение подходит".
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (username, password_hash, created_at, avatar) VALUES ($1, $2, $3, $4)`)).
		WithArgs("alice", sqlmock.AnyArg(), sqlmock.AnyArg(), sql.NullString{String: "avatar.png", Valid: true}).
		WillReturnResult(sqlmock.NewResult(1, 1)) // эмулируем успешный INSERT

	// Пытаемся зарегистрировать нового пользователя
	err := store.Register("alice", "secret", "avatar.png")

	// Проверяем, что ошибок не возникло
	assert.NoError(t, err)
	// Проверяем, что все ожидаемые SQL-запросы действительно были вызваны
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRegister_EmptyUsernameOrPassword(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Случай: пустой логин
	err := store.Register("", "secret", "")
	assert.EqualError(t, err, "username and password are required")

	// Случай: пустой пароль
	err = store.Register("bob", "", "")
	assert.EqualError(t, err, "username and password are required")
}

func TestRegister_TooLongUsername(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Создаём слишком длинный логин (больше 24 символов)
	longName := "this_is_way_too_long_username"
	err := store.Register(longName, "secret", "")

	// Проверяем, что вернулась ожидаемая ошибка
	assert.EqualError(t, err, "username too long (max 24)")
}

func TestRegister_UniqueViolation(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Эмулируем ситуацию: база вернула ошибку уникальности (duplicate key)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (username, password_hash, created_at, avatar) VALUES ($1, $2, $3, $4)`)).
		WillReturnError(errors.New("unique constraint"))

	err := store.Register("alice", "secret", "")

	// Метод должен вернуть читаемое сообщение
	assert.EqualError(t, err, "username already exists")
}

// --- ТЕСТЫ ДЛЯ Authenticate ---
// Authenticate — проверяет логин/пароль пользователя

func TestAuthenticate_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Генерируем bcrypt-хэш для пароля "secret"
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)

	// Эмулируем успешный SELECT: база возвращает хэш пароля
	mock.ExpectQuery(`SELECT password_hash FROM users WHERE username=`).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))

	// Проверяем вход с правильным паролем
	ok := store.Authenticate("alice", "secret")

	// Должно быть true
	assert.True(t, ok)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Сохраняем в базе хэш правильного пароля "secret"
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)

	mock.ExpectQuery(`SELECT password_hash FROM users WHERE username=`).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"password_hash"}).AddRow(string(hash)))

	// Пробуем авторизоваться с неправильным паролем
	ok := store.Authenticate("alice", "wrongpass")

	// Ожидаем, что результат — false
	assert.False(t, ok)
}

func TestAuthenticate_UserNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Эмулируем ситуацию: база не нашла пользователя
	mock.ExpectQuery(`SELECT password_hash FROM users WHERE username=`).
		WithArgs("bob").
		WillReturnError(sql.ErrNoRows)

	ok := store.Authenticate("bob", "whatever")

	// Авторизация должна провалиться
	assert.False(t, ok)
}

// --- ТЕСТЫ ДЛЯ GetAvatar ---
// GetAvatar — возвращает путь к аватарке пользователя

func TestGetAvatar_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Эмулируем SELECT, который возвращает строку с аватаркой
	mock.ExpectQuery(`SELECT avatar FROM users WHERE username=`).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow("avatar.png"))

	avatar := store.GetAvatar("alice")

	// Проверяем, что вернулся корректный путь
	assert.Equal(t, "avatar.png", avatar)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAvatar_NoAvatar(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := &user.Store{Db: db}

	// Эмулируем ситуацию: пользователь не найден
	// Scan вернёт ошибку, метод её проигнорирует и вернёт пустую строку
	mock.ExpectQuery(`SELECT avatar FROM users WHERE username=`).
		WithArgs("bob").
		WillReturnError(sql.ErrNoRows)

	avatar := store.GetAvatar("bob")

	// Если аватарки нет, метод всегда возвращает ""
	assert.Equal(t, "", avatar)
}
