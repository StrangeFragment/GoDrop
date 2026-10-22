package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"GoDrop/storage"
	"GoDrop/web"
)

// Handler обслуживает HTTP-запросы GoDrop.
type Handler struct {
	Storage           *storage.Storage
	AllowedExtensions []string
	MaxUploadBytes    int64
	MultipartMemory   int64

	// AccessCode — код доступа к странице (пустая строка — без кода).
	AccessCode string

	// OnFilesSaved вызывается после успешной загрузки файлов,
	// чтобы показать уведомление в трее.
	OnFilesSaved func(count int)

	// salt — случайная строка процесса, из которой считается cookie.
	salt string
}

// UploadResult — данные страницы с результатами загрузки.
type UploadResult struct {
	Uploaded []string
	Rejected []RejectedFile
}

// RejectedFile — файл, который не удалось принять.
type RejectedFile struct {
	Name   string
	Reason string
}

// ErrorPageData — данные страницы ошибки.
type ErrorPageData struct {
	Code    int
	Title   string
	Message string
}

// IndexData — данные главной страницы.
type IndexData struct {
	Accept      string
	MaxFileSize string
	HasCode     bool
	Recent      []storage.FileInfo
}

// fileListLimit — сколько последних файлов показывать на главной.
const fileListLimit = 5

var pageStyle = template.CSS(`
	body {
		background: #0f0f1a;
		color: #e8e8f0;
		font-family: 'Segoe UI', Arial, sans-serif;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 24px 16px 40px;
		margin: 0;
		min-height: 100vh;
		box-sizing: border-box;
	}

	h1 {
		color: #04AA6D;
		margin: 0 0 8px;
		text-align: center;
		font-size: 30px;
	}

	h2, h3 {
		color: #e8e8f0;
	}

	p {
		color: #8888aa;
		font-size: 14px;
		margin-bottom: 24px;
	}

	main {
		background: #0f0f1a;
		width: 100%;
		max-width: 520px;
		margin: 0 auto;
		padding: 8px 8px 24px;
		box-sizing: border-box;
		border-radius: 16px;
	}

	.card {
		background: #191927;
		border-radius: 12px;
		padding: 18px;
		margin-top: 20px;
	}

	ul {
		margin: 14px 0 0;
		padding: 0;
		list-style: none;
	}

	li {
		padding: 8px 10px;
		margin-bottom: 6px;
		background: #0f0f1a;
		border-radius: 6px;
		color: #c8c8d8;
		font-size: 14px;
		overflow-wrap: anywhere;
	}

	a {
		text-decoration: none;
	}

	button, .file-button {
		display: block;
		width: 100%;
		box-sizing: border-box;
		margin-top: 16px;
		padding: 14px;
		font-size: 17px;
		text-align: center;
		border: 0;
		border-radius: 8px;
		background: #1769ff;
		color: white;
		cursor: pointer;
	}

	button:hover:not(:disabled), .file-button:hover {
		background: #0d55d4;
	}

	button:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.file-button {
		background: #04AA6D;
	}

	.file-button:hover {
		background: #078957;
	}

	#fileInput {
		display: none;
	}

	#fileList {
		margin: 14px 0 0;
		padding: 0;
		list-style: none;
		color: #c8c8d8;
		font-size: 14px;
	}

	#fileList li {
		padding: 8px 10px;
		margin-bottom: 6px;
		background: #191927;
		border-radius: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.hint {
		text-align: center;
		font-size: 13px;
		color: #8888aa;
		margin: 12px 0 0;
	}

	.error-code {
		font-size: 72px;
		font-weight: 700;
		color: #04AA6D;
		text-align: center;
		margin: 10px 0;
	}

	.error-message {
		text-align: center;
	}

	footer {
		margin-top: 24px;
		color: #8888aa;
		font-size: 13px;
		text-align: center;
	}
`)

var templateFuncs = template.FuncMap{
	"style": func() template.CSS {
		return pageStyle
	},
	"size": humanSize,
}

var indexTemplate = template.Must(
	template.New("index").Funcs(templateFuncs).Parse(string(web.IndexHTML)),
)

var resultTemplate = template.Must(template.New("result").Funcs(templateFuncs).Parse(`
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>GoDrop! — результаты загрузки</title>
	<link rel="icon" href="/favicon.ico" type="image/x-icon">
	<style>{{style}}</style>
</head>
<body>
	<main>
		<h1>GoDrop!</h1>

		<div class="card">
			<h3>Результаты загрузки</h3>

			<h2>Успешно загружено: {{len .Uploaded}}</h2>

			<ul>
				{{range .Uploaded}}
					<li>{{.}}</li>
				{{else}}
					<li>Нет успешно загруженных файлов</li>
				{{end}}
			</ul>

			{{if .Rejected}}
				<h2>Отклонено: {{len .Rejected}}</h2>

				<ul>
					{{range .Rejected}}
						<li>{{.Name}} — {{.Reason}}</li>
					{{end}}
				</ul>
			{{end}}

			<a href="/">
				<button type="button">Загрузить еще...</button>
			</a>
		</div>
	</main>

	<footer>
		Created by <strong>StrangeFragment</strong>, 2©26
	</footer>
</body>
</html>
`))

var errorTemplate = template.Must(template.New("error").Funcs(templateFuncs).Parse(`
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.Code}} — GoDrop!</title>
	<link rel="icon" href="/favicon.ico" type="image/x-icon">
	<style>{{style}}</style>
</head>
<body>
	<main>
		<h1>GoDrop!</h1>

		<div class="card">
			<div class="error-code">{{.Code}}</div>
			<p class="error-message">{{.Title}}</p>
			<p class="error-message">{{.Message}}</p>

			<a href="/">
				<button type="button">Вернуться на главную</button>
			</a>
		</div>
	</main>

	<footer>
		Created by <strong>StrangeFragment</strong>, 2©26
	</footer>
</body>
</html>
`))

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.ErrorPage(
			w,
			http.StatusMethodNotAllowed,
			"Метод не разрешен",
			"Для этой страницы поддерживается только GET-запрос.",
		)
		return
	}

	// Если Index зарегистрирован на маршруте "/", не позволяем ему
	// отображать главную страницу для неизвестных URL.
	if r.URL.Path != "/" {
		h.ErrorPage(
			w,
			http.StatusNotFound,
			"Страница не найдена",
			"Запрошенный адрес не существует.",
		)
		return
	}

	if !h.requireAccess(w, r) {
		return
	}

	data := IndexData{
		Accept:      strings.Join(h.AllowedExtensions, ","),
		MaxFileSize: humanSize(h.MaxUploadBytes),
		HasCode:     h.AccessCode != "",
	}

	// Последние принятые файлы: видно, что передача дошла до компьютера.
	if recent, err := h.Storage.List(fileListLimit); err == nil {
		data.Recent = recent
	} else {
		log.Printf("не удалось прочитать список файлов: %v", err)
	}

	h.setPageHeaders(w)
	w.WriteHeader(http.StatusOK)

	if err := indexTemplate.Execute(w, data); err != nil {
		log.Printf("ошибка отрисовки главной страницы: %v", err)
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.ErrorPage(
			w,
			http.StatusMethodNotAllowed,
			"Метод не разрешен",
			"Загрузка файлов выполняется через POST-запрос.",
		)
		return
	}

	if !h.requireAccess(w, r) {
		return
	}

	// Ограничиваем размер запроса, чтобы одно устройство не забило
	// диск и не съело память.
	if h.MaxUploadBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, h.MaxUploadBytes)
	}

	memory := h.MultipartMemory
	if memory <= 0 {
		memory = multipartMemory
	}

	if err := r.ParseMultipartForm(memory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.ErrorPage(
				w,
				http.StatusRequestEntityTooLarge,
				"Файлы слишком большие",
				fmt.Sprintf(
					"За один раз можно передать не больше %s.",
					humanSize(h.MaxUploadBytes),
				),
			)
			return
		}

		h.ErrorPage(
			w,
			http.StatusBadRequest,
			"Не удалось обработать загрузку",
			"Проверьте выбранные файлы и попробуйте еще раз.",
		)
		return
	}

	if r.MultipartForm != nil {
		defer func() {
			if err := r.MultipartForm.RemoveAll(); err != nil {
				log.Printf("не удалось удалить временные файлы загрузки: %v", err)
			}
		}()
	}

	files := r.MultipartForm.File["files"]

	if len(files) == 0 {
		h.ErrorPage(
			w,
			http.StatusBadRequest,
			"Файлы не выбраны",
			"Выберите хотя бы один файл для загрузки.",
		)
		return
	}

	result := UploadResult{
		Uploaded: make([]string, 0, len(files)),
	}

	for _, header := range files {
		if !isAllowed(header.Filename, h.AllowedExtensions) {
			result.Rejected = append(result.Rejected, RejectedFile{
				Name:   header.Filename,
				Reason: "недопустимый тип файла",
			})
			continue
		}

		src, err := header.Open()
		if err != nil {
			log.Printf("не удалось открыть принятый файл %q: %v", header.Filename, err)

			result.Rejected = append(result.Rejected, RejectedFile{
				Name:   header.Filename,
				Reason: "ошибка чтения файла",
			})
			continue
		}

		savedName, saveErr := h.Storage.SaveStream(header.Filename, src)
		_ = src.Close()

		if saveErr != nil {
			log.Printf("не удалось сохранить файл %q: %v", header.Filename, saveErr)

			result.Rejected = append(result.Rejected, RejectedFile{
				Name:   header.Filename,
				Reason: "ошибка записи на диск",
			})
			continue
		}

		log.Printf("сохранён файл: %s", savedName)
		result.Uploaded = append(result.Uploaded, savedName)
	}

	if len(result.Uploaded) > 0 && h.OnFilesSaved != nil {
		h.OnFilesSaved(len(result.Uploaded))
	}

	h.setPageHeaders(w)
	w.WriteHeader(http.StatusOK)

	if err := resultTemplate.Execute(w, result); err != nil {
		log.Printf("ошибка отрисовки страницы результатов: %v", err)
	}
}

func (h *Handler) ErrorPage(
	w http.ResponseWriter,
	status int,
	title string,
	message string,
) {
	data := ErrorPageData{
		Code:    status,
		Title:   title,
		Message: message,
	}

	h.setPageHeaders(w)
	w.WriteHeader(status)

	if err := errorTemplate.Execute(w, data); err != nil {
		log.Printf("ошибка отрисовки страницы ошибки: %v", err)
	}
}

func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(
			w,
			"метод не разрешен",
			http.StatusMethodNotAllowed,
		)
		return
	}

	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set(
		"Cache-Control",
		"public, max-age=86400",
	)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(web.FaviconICO)
}

// setPageHeaders выставляет заголовки для HTML-страниц.
func (h *Handler) setPageHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Страница должна обновляться после смены настроек,
	// а не браться из кеша телефона.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// humanSize печатает размер в удобном виде.
func humanSize(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d Б", bytes)
	}

	value := float64(bytes)

	for _, suffix := range []string{"КиБ", "МиБ", "ГиБ", "ТиБ"} {
		value /= unit

		if value < unit {
			return fmt.Sprintf("%.0f %s", value, suffix)
		}
	}

	return fmt.Sprintf("%.0f ПиБ", value/unit)
}
