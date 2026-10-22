package server

import (
	"net/http"

	"GoDrop/web"
)

// serveBytes отдаёт заранее известное содержимое с кешированием.
func serveBytes(w http.ResponseWriter, r *http.Request, contentType string, data []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write(data)
}

// Manifest отдаёт манифест веб-приложения: без него ярлык на телефоне
// получает случайную иконку и открывается как обычная вкладка браузера.
func (h *Handler) Manifest(w http.ResponseWriter, r *http.Request) {
	serveBytes(
		w,
		r,
		"application/manifest+json; charset=utf-8",
		[]byte(web.Manifest),
	)
}

// Icon192 отдаёт иконку 192×192.
func (h *Handler) Icon192(w http.ResponseWriter, r *http.Request) {
	serveBytes(w, r, "image/png", web.Icon192)
}

// Icon512 отдаёт иконку 512×512.
func (h *Handler) Icon512(w http.ResponseWriter, r *http.Request) {
	serveBytes(w, r, "image/png", web.Icon512)
}

// AppleTouchIcon отдаёт иконку для ярлыка на iPhone/iPad.
func (h *Handler) AppleTouchIcon(w http.ResponseWriter, r *http.Request) {
	serveBytes(w, r, "image/png", web.AppleTouchIcon)
}
