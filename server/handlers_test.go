package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GoDrop/storage"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	return &Handler{
		Storage:           storage.New(t.TempDir()),
		AllowedExtensions: []string{".jpg", ".png", ".txt"},
		MaxUploadBytes:    MaxUploadBytes,
		MultipartMemory:   multipartMemory,
	}
}

func TestIsAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{".jpg", " .PNG ", "txt"}

	cases := map[string]bool{
		"photo.jpg":     true,
		"photo.JPG":     true,
		"photo.png":     true,
		"photo.txt":     true,
		"photo.exe":     false,
		"photo":         false,
		"photo.jpg.exe": false,
		"":              false,
	}

	for filename, want := range cases {
		if got := isAllowed(filename, allowed); got != want {
			t.Errorf("isAllowed(%q) = %v, want %v", filename, got, want)
		}
	}
}

func TestIndexRendersTemplate(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "GoDrop!") {
		t.Error("на странице нет заголовка GoDrop!")
	}

	if !strings.Contains(body, `accept=".jpg,.png,.txt"`) {
		t.Errorf("в форме нет accept из настроек: %s", body)
	}

	if !strings.Contains(body, `action="/upload"`) {
		t.Error("форма не отправляет файлы на /upload")
	}
}

func TestIndexUnknownPath(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/unknown", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("код ответа = %d, want 404", recorder.Code)
	}
}

func TestIndexRejectsPost(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodPost, "/", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код ответа = %d, want 405", recorder.Code)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.Health(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}

	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Errorf("тело ответа: %s", recorder.Body.String())
	}
}

func multipartBody(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()

	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)

	for name, content := range files {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}

		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	return buffer, writer.FormDataContentType()
}

func TestUploadSavesAllowedFiles(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	body, contentType := multipartBody(t, map[string]string{
		"photo.jpg": "image",
		"notes.txt": "text",
		"evil.exe":  "binary",
	})

	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.Header.Set("Content-Type", contentType)

	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	response := recorder.Body.String()

	if !strings.Contains(response, "Успешно загружено: 2") {
		t.Errorf("ожидалось 2 загруженных файла: %s", response)
	}

	if !strings.Contains(response, "Отклонено: 1") {
		t.Errorf("ожидался 1 отклонённый файл: %s", response)
	}

	if !strings.Contains(response, "недопустимый тип файла") {
		t.Error("не указана причина отклонения")
	}
}

func TestUploadWithoutFiles(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	body, contentType := multipartBody(t, map[string]string{})

	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.Header.Set("Content-Type", contentType)

	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("код ответа = %d, want 400", recorder.Code)
	}
}

func TestUploadOverLimit(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	handler.MaxUploadBytes = 32

	body, contentType := multipartBody(t, map[string]string{
		"big.txt": strings.Repeat("x", 4096),
	})

	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.Header.Set("Content-Type", contentType)

	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("код ответа = %d, want 413", recorder.Code)
	}
}

func TestUploadWrongMethod(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	handler.Upload(recorder, httptest.NewRequest(http.MethodGet, "/upload", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("код ответа = %d, want 405", recorder.Code)
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

	cases := map[int64]string{
		512:     "512 Б",
		8 << 30: "8 ГиБ",
		1536:    "2 КиБ",
		5 << 20: "5 МиБ",
	}

	for size, want := range cases {
		if got := humanSize(size); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", size, got, want)
		}
	}
}
