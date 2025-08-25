package web

import "net/http"

// =========================
// withJSON задаёт заголовки JSON
// =========================
func withJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
}
