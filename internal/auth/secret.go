package auth

// Secret для подписи JWT
var Secret []byte

// InitSecret устанавливает секрет для JWT
func InitSecret(secret []byte) {
	Secret = secret
}
