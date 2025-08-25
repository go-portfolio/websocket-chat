package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-portfolio/websocket-chat/internal/auth"
	"github.com/go-portfolio/websocket-chat/internal/user"
)

// =========================
// Регистрация пользователя
// POST /api/register
// =========================
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	// Парсим multipart/form-data
	err := r.ParseMultipartForm(10 << 20) // 10MB лимит
	if err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	var cred user.Credentials
	cred.Username = r.FormValue("username")
	cred.Password = r.FormValue("password")

	file, handler, err := r.FormFile("avatar")
	var avatarURL string

	if err == nil { // файл передан
		defer file.Close()

		// создаём папку, если нет
		os.MkdirAll("../../uploads", os.ModePerm)

		// уникальное имя
		filename := fmt.Sprintf("../../uploads/%d_%s", time.Now().Unix(), handler.Filename)
		dst, err := os.Create(filename)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot save file"})
			return
		}
		defer dst.Close()

		if _, err = io.Copy(dst, file); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot write file"})
			return
		}

		avatarURL = fmt.Sprintf("/uploads/%d_%s", time.Now().Unix(), handler.Filename)
	}

	// Регистрируем пользователя
	if err := Users.Register(cred.Username, cred.Password, avatarURL); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// =========================
// Логин пользователя
// POST /api/login
// =========================
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	withJSON(w)

	var cred user.Credentials
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	if !Users.Authenticate(cred.Username, cred.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}

	token, err := auth.IssueJWT(cred.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to issue token"})
		return
	}

	cookie := http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   24 * 60 * 60,
	}
	http.SetCookie(w, &cookie)

	var avatar = Users.GetAvatar(cred.Username)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "avatar": avatar})
}

// =========================
// IndexHandler
// =========================
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join("..", "..", "internal", "web", "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
