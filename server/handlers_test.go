package server

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	if !strings.Contains(body, `<a href="/files" class="action-button">Все файлы</a>`) {
		t.Error("ссылка «Все файлы» не использует общий стиль кнопок")
	}

	if !strings.Contains(body, "['KB', 'MB', 'GB']") {
		t.Error("размеры файлов в браузере должны отображать KB, MB и GB")
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
		512:     "512 B",
		1024:    "1 KB",
		8 << 30: "8 GB",
		1536:    "2 KB",
		5 << 20: "5 MB",
	}

	for size, want := range cases {
		if got := humanSize(size); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", size, got, want)
		}
	}
}

func newProtectedHandler(t *testing.T, code string) *Handler {
	t.Helper()

	handler := newTestHandler(t)
	handler.AccessCode = code

	if err := handler.initAccess(); err != nil {
		t.Fatalf("initAccess() error: %v", err)
	}

	return handler
}

func TestAccessRequiredWithoutCode(t *testing.T) {
	t.Parallel()

	handler := newProtectedHandler(t, "123456")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("код ответа = %d, want 401", recorder.Code)
	}

	if !strings.Contains(recorder.Body.String(), "Нужен код доступа") {
		t.Error("не показана форма ввода кода")
	}
}

func TestAccessByLinkSetsCookie(t *testing.T) {
	t.Parallel()

	handler := newProtectedHandler(t, "123456")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/?k=123456", nil))

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("код ответа = %d, want 303", recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("cookie с доступом не выставлена")
	}

	// В cookie не должно быть самого кода.
	if strings.Contains(cookies[0].Value, "123456") {
		t.Error("код доступа попал в cookie в открытом виде")
	}

	// Повторный запрос с cookie проходит.
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookies[0])

	recorder = httptest.NewRecorder()
	handler.Index(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("с cookie код ответа = %d, want 200", recorder.Code)
	}
}

func TestAccessWrongCode(t *testing.T) {
	t.Parallel()

	handler := newProtectedHandler(t, "123456")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/?k=000000", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("код ответа = %d, want 401", recorder.Code)
	}

	if !strings.Contains(recorder.Body.String(), "Код не подошёл") {
		t.Error("не показано сообщение о неверном коде")
	}
}

func TestAccessDisabledByDefault(t *testing.T) {
	t.Parallel()

	handler := newProtectedHandler(t, "")

	recorder := httptest.NewRecorder()
	handler.Index(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}
}

func TestSanitizeNext(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                      "/",
		"/files":                "/files",
		"/files?x=1":            "/files?x=1",
		"http://evil.example/x": "/",
		"//evil.example/x":      "/",
		"files":                 "/",
		"javascript:alert(1)":   "/",
	}

	for input, want := range cases {
		if got := sanitizeNext(input); got != want {
			t.Errorf("sanitizeNext(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFilesPageListsUploaded(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	if _, err := handler.Storage.SaveStream("report.pdf", strings.NewReader("data")); err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.Files(recorder, httptest.NewRequest(http.MethodGet, "/files", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "report.pdf") {
		t.Errorf("в списке нет файла: %s", body)
	}

	if !strings.Contains(body, "/files/download?f=") {
		t.Error("нет ссылки на скачивание")
	}

	if !strings.Contains(body, `<p class="page-subtitle">Принятые файлы на компьютере</p>`) {
		t.Error("заголовок списка принятых файлов не оформлен по центру")
	}
}

func TestFilesPageSearchAndPagination(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	for i := 0; i < filesPageSize+1; i++ {
		name := fmt.Sprintf("Фото-%03d.jpg", i)
		if _, err := handler.Storage.SaveStream(name, strings.NewReader("x")); err != nil {
			t.Fatalf("SaveStream(%q) error: %v", name, err)
		}
	}
	if _, err := handler.Storage.SaveStream("notes.txt", strings.NewReader("not a photo")); err != nil {
		t.Fatalf("SaveStream(notes.txt) error: %v", err)
	}

	search := "ФОТО"
	request := httptest.NewRequest(
		http.MethodGet,
		"/files?q="+url.QueryEscape(search)+"&page=2",
		nil,
	)
	recorder := httptest.NewRecorder()
	handler.Files(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `value="ФОТО"`) {
		t.Error("поисковый запрос не сохранён в поле поиска")
	}
	if !strings.Contains(body, "Найдено файлов: 51 — 51 B") {
		t.Error("сводка должна учитывать все совпадения, а не только текущую страницу")
	}
	if !strings.Contains(body, "Показано 51–51 из 51 · страница 2 из 2") {
		t.Error("неверные диапазон или номер страницы")
	}
	if !strings.Contains(body, "Фото-000.jpg") {
		t.Error("последний файл не отображается на второй странице")
	}
	if strings.Contains(body, "Фото-001.jpg") {
		t.Error("файл с первой страницы попал на вторую")
	}
	if strings.Contains(body, "notes.txt") {
		t.Error("поиск не отфильтровал файл, не совпадающий с запросом")
	}
	if !strings.Contains(body, `href="/files?page=1&amp;q=%D0%A4%D0%9E%D0%A2%D0%9E"`) {
		t.Error("ссылка «Назад» не сохраняет поисковый запрос")
	}
}

func TestFilesPageNoSearchResults(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if _, err := handler.Storage.SaveStream("report.pdf", strings.NewReader("data")); err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.Files(
		recorder,
		httptest.NewRequest(http.MethodGet, "/files?q=missing", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "По запросу «missing» ничего не найдено") {
		t.Error("не показано сообщение об отсутствии совпадений")
	}
	if !strings.Contains(body, `href="/files">Сбросить поиск</a>`) {
		t.Error("не показана ссылка для сброса поиска")
	}
	if strings.Contains(body, "report.pdf") {
		t.Error("файл попал в список при несовпадающем поиске")
	}
}

func TestDownloadFile(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	if _, err := handler.Storage.SaveStream("фото 1.jpg", strings.NewReader("image-data")); err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	files, err := handler.Storage.List(1)
	if err != nil || len(files) != 1 {
		t.Fatalf("List() = %v, %v", files, err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"/files/download?f="+url.QueryEscape(files[0].RelPath),
		nil,
	)

	recorder := httptest.NewRecorder()
	handler.Download(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("код ответа = %d, want 200", recorder.Code)
	}

	if recorder.Body.String() != "image-data" {
		t.Errorf("содержимое = %q, want %q", recorder.Body.String(), "image-data")
	}

	disposition := recorder.Header().Get("Content-Disposition")

	if !strings.Contains(disposition, "filename*=UTF-8''") {
		t.Errorf("Content-Disposition = %q", disposition)
	}
}

func TestDownloadRejectsTraversal(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	request := httptest.NewRequest(
		http.MethodGet,
		"/files/download?f=../../secret.txt",
		nil,
	)

	recorder := httptest.NewRecorder()
	handler.Download(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("код ответа = %d, want 404", recorder.Code)
	}
}

func TestHumanDate(t *testing.T) {
	t.Parallel()

	if got := humanDate("2026-10-22"); got != "22 октября 2026" {
		t.Errorf("humanDate() = %q", got)
	}

	if got := humanDate("не дата"); got != "не дата" {
		t.Errorf("humanDate() для мусора = %q", got)
	}
}

func TestNotifyAfterUpload(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)

	var saved int
	handler.OnFilesSaved = func(count int) { saved = count }

	body, contentType := multipartBody(t, map[string]string{
		"photo.jpg": "image",
		"notes.txt": "text",
	})

	request := httptest.NewRequest(http.MethodPost, "/upload", body)
	request.Header.Set("Content-Type", contentType)

	recorder := httptest.NewRecorder()
	handler.Upload(recorder, request)

	if saved != 2 {
		t.Errorf("OnFilesSaved получил %d, want 2", saved)
	}
}
