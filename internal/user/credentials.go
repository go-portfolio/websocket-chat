package user

// Credentials — структура для логина/регистрации
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Avatar   string `json:"avatar"`
}
