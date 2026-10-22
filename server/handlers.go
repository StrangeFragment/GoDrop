package server

import (
	"encoding/json"
	"html/template"
	"net/http"

	"GoDrop/storage"
	"GoDrop/web"
)

type Handler struct {
	Storage           *storage.Storage
	AllowedExtensions []string
}

type UploadResult struct {
	Uploaded []string
	Rejected []string
}

type ErrorPageData struct {
	Code    int
	Title   string
	Message string
}

var pageStyle = template.CSS(`
	body {
		background: #0f0f1a;
		color: #e8e8f0;
		font-family: 'Segoe UI', Arial, sans-serif;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 40px 20px;
		margin: 0;
		min-height: 100vh;
		box-sizing: border-box;
	}

	h1 {
		color: #04AA6D;
		margin-bottom: 8px;
		text-align: center;
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
		padding: 24px;
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
		background: #191927;
		border-radius: 6px;
		color: #c8c8d8;
		font-size: 14px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	a {
		text-decoration: none;
	}

	button {
		width: 100%;
		box-sizing: border-box;
		margin-top: 16px;
		padding: 14px;
		font-size: 17px;
		border: 0;
		border-radius: 8px;
		background: #1769ff;
		color: white;
		cursor: pointer;
	}

	button:hover {
		background: #0d55d4;
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

var resultTemplate = template.Must(template.New("result").Funcs(template.FuncMap{
	"style": func() template.CSS {
		return pageStyle
	},
}).Parse(`
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>GoDrop!</title>
	<link rel="icon" href="/favicon.ico" type="image/x-icon">
	<style>{{style}}</style>
</head>
<body>
	<main>
		<h1>GoDrop!</h1>

		<p style="text-align: center;">
			программа для Windows, которая работает в системном трее
			и позволяет передавать фотографии и файлы с телефона
			на компьютер через локальную сеть.
		</p>

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
						<li>{{.}}</li>
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

var errorTemplate = template.Must(template.New("error").Funcs(template.FuncMap{
	"style": func() template.CSS {
		return pageStyle
	},
}).Parse(`
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
	if r.Method != http.MethodGet {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(web.IndexHTML)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
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

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.ErrorPage(
			w,
			http.StatusBadRequest,
			"Не удалось обработать загрузку",
			"Проверьте выбранные файлы и попробуйте еще раз.",
		)
		return
	}

	result := UploadResult{}

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

	for _, header := range files {
		if !isAllowed(header.Filename, h.AllowedExtensions) {
			result.Rejected = append(
				result.Rejected,
				header.Filename,
			)
			continue
		}

		src, err := header.Open()
		if err != nil {
			result.Rejected = append(
				result.Rejected,
				header.Filename,
			)
			continue
		}

		savedName, saveErr := h.Storage.SaveStream(
			header.Filename,
			src,
		)

		_ = src.Close()

		if saveErr != nil {
			result.Rejected = append(
				result.Rejected,
				header.Filename,
			)
			continue
		}

		result.Uploaded = append(result.Uploaded, savedName)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_ = resultTemplate.Execute(w, result)
}

func (h *Handler) ErrorPage(
	w http.ResponseWriter,
	status int,
	title string,
	message string,
) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	data := ErrorPageData{
		Code:    status,
		Title:   title,
		Message: message,
	}

	_ = errorTemplate.Execute(w, data)
}

func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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
