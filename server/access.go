package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const (
	// accessCookieName — cookie, подтверждающая ввод кода доступа.
	accessCookieName = "godrop_access"

	// accessQueryParam — параметр ссылки с кодом доступа (?k=123456).
	accessQueryParam = "k"

	// accessCookieMaxAge — сколько помнить телефон (30 дней).
	accessCookieMaxAge = 30 * 24 * 60 * 60

	// accessSaltLength — длина случайной "соли" процесса.
	accessSaltLength = 16
)

// initAccess готовит обработчик к работе с кодом доступа:
// генерирует соль и, если включена проверка, пишет в лог подсказку.
func (h *Handler) initAccess() error {
	salt := make([]byte, accessSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("не удалось подготовить код доступа: %w", err)
	}

	h.salt = hex.EncodeToString(salt)

	if h.AccessCode != "" {
		log.Printf("включён код доступа: страница откроется только по ссылке с кодом")
	}

	return nil
}

// accessToken считает значение cookie для текущего кода доступа.
// Сам код в cookie не попадает, поэтому его нельзя подсмотреть
// в настройках браузера телефона.
func (h *Handler) accessToken() string {
	sum := sha256.Sum256([]byte("GoDrop|" + h.salt + "|" + h.AccessCode))

	return hex.EncodeToString(sum[:])
}

// accessGranted проверяет cookie или параметр ссылки.
func (h *Handler) accessGranted(r *http.Request) bool {
	if h.AccessCode == "" {
		return true
	}

	if cookie, err := r.Cookie(accessCookieName); err == nil {
		expected := h.accessToken()

		if subtle.ConstantTimeCompare(
			[]byte(cookie.Value),
			[]byte(expected),
		) == 1 {
			return true
		}
	}

	provided := r.URL.Query().Get(accessQueryParam)
	if provided == "" {
		return false
	}

	return subtle.ConstantTimeCompare(
		[]byte(provided),
		[]byte(h.AccessCode),
	) == 1
}

// requireAccess пропускает запрос дальше или показывает форму ввода кода.
// Возвращает false, если запрос уже обработан.
func (h *Handler) requireAccess(w http.ResponseWriter, r *http.Request) bool {
	if h.accessGranted(r) {
		// Код пришёл ссылкой — запоминаем его в cookie и убираем
		// из адресной строки, чтобы он не остался в истории браузера.
		if r.URL.Query().Get(accessQueryParam) != "" {
			h.setAccessCookie(w)

			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				target := "/"
				if next := r.URL.Query().Get("next"); next != "" {
					target = sanitizeNext(next)
				}

				http.Redirect(w, r, target, http.StatusSeeOther)

				return false
			}
		}

		return true
	}

	h.showAccessForm(w, r)

	return false
}

// setAccessCookie запоминает введённый код в браузере телефона.
func (h *Handler) setAccessCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessCookieName,
		Value:    h.accessToken(),
		Path:     "/",
		MaxAge:   accessCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// showAccessForm показывает страницу ввода кода.
func (h *Handler) showAccessForm(w http.ResponseWriter, r *http.Request) {
	data := accessPageData{
		Code:    http.StatusUnauthorized,
		Title:   "Нужен код доступа",
		Message: "Введите код, который показан в программе на компьютере.",
		Next:    sanitizeNext(r.URL.RequestURI()),
		Error:   r.URL.Query().Get(accessQueryParam) != "",
	}

	h.setPageHeaders(w)
	w.WriteHeader(http.StatusUnauthorized)

	if err := accessTemplate.Execute(w, data); err != nil {
		log.Printf("ошибка отрисовки страницы кода доступа: %v", err)
	}
}

// sanitizeNext оставляет только локальные пути: открытые редиректы
// на чужие сайты недопустимы.
func sanitizeNext(value string) string {
	if value == "" {
		return "/"
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}

	if !strings.HasPrefix(parsed.Path, "/") {
		return "/"
	}

	// Свою же форму ввода кода в качестве "куда вернуться" не берём.
	if parsed.Path == "" {
		return "/"
	}

	parsed.Scheme = ""
	parsed.Host = ""

	return parsed.String()
}
