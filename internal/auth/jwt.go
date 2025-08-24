package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Secret — глобальный секрет для подписи JWT-токенов.
// ⚠️ В реальном проекте рекомендуется брать из переменной окружения или безопасного хранилища.
var Secret []byte

// InitSecret устанавливает секрет для JWT.
// Обычно вызывается при старте сервера.
func InitSecret(secret []byte) {
	Secret = secret
}

// IssueJWT создаёт JWT-токен для указанного username.
// Токен содержит стандартные поля:
// - "sub" — субъект (username)
// - "iat" — время выпуска (issued at)
// - "exp" — время истечения (expiration), здесь 24 часа
func IssueJWT(username string) (string, error) {
	claims := jwt.MapClaims{
		"sub": username,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}
	// Создаём новый JWT с алгоритмом HMAC SHA256
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Подписываем токен с секретом
	return token.SignedString(Secret)
}

// ParseJWT проверяет токен и возвращает username.
// 1. Проверяет подпись токена с помощью Secret.
// 2. Проверяет метод подписи (только HMAC).
// 3. Проверяет валидность токена.
// 4. Извлекает claims и возвращает "sub" (username).
func ParseJWT(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Проверяем, что метод подписи ожидаемый (HMAC)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return Secret, nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	// Приводим claims к типу MapClaims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}

	// Получаем username из поля "sub"
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", fmt.Errorf("missing subject")
	}
	return sub, nil
}
