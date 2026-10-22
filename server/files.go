package server

import (
	"fmt"
	"html/template"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"GoDrop/storage"
)

const (
	// filesPageLimit — сколько файлов показывать на странице.
	filesPageLimit = 500
)

// fileGroup — файлы за один день.
type fileGroup struct {
	Date  string
	Label string
	Files []storage.FileInfo
}

// filesPageData — данные страницы "Принятые файлы".
type filesPageData struct {
	Groups []fileGroup
	Total  int
	Size   int64
	Empty  bool
}

// accessPageData — данные страницы ввода кода доступа.
type accessPageData struct {
	Code    int
	Title   string
	Message string
	Next    string
	Error   bool
}

var pageFuncs = template.FuncMap{
	"style": func() template.CSS {
		return pageStyle
	},
	"size": humanSize,
	"date": func(t time.Time) string {
		return t.Format("15:04")
	},
}

var accessTemplate = template.Must(template.New("access").Funcs(pageFuncs).Parse(`
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Код доступа — GoDrop!</title>
	<link rel="icon" href="/favicon.ico" type="image/x-icon">
	<link rel="apple-touch-icon" href="/apple-touch-icon.png">
	<style>{{style}}</style>
</head>
<body>
	<main>
		<h1>GoDrop!</h1>

		<div class="card">
			<h3>{{.Title}}</h3>
			<p class="error-message">{{.Message}}</p>

			{{if .Error}}
				<p class="error-message" style="color: #ff8080;">Код не подошёл. Попробуйте ещё раз.</p>
			{{end}}

			<form action="/" method="get">
				<input type="hidden" name="next" value="{{.Next}}">
				<input
					type="text"
					name="k"
					inputmode="numeric"
					autocomplete="one-time-code"
					placeholder="Код доступа"
					autofocus
					style="width: 100%; box-sizing: border-box; padding: 14px; font-size: 20px; text-align: center; letter-spacing: 4px; border: 0; border-radius: 8px; background: #0f0f1a; color: #e8e8f0;"
				>
				<button type="submit">Войти</button>
			</form>

			<p class="hint">Код показан в окне с QR-кодом на компьютере.</p>
		</div>
	</main>

	<footer>
		Created by <strong>StrangeFragment</strong>, 2©26
	</footer>
</body>
</html>
`))

var filesTemplate = template.Must(template.New("files").Funcs(pageFuncs).Parse(`
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Принятые файлы — GoDrop!</title>
	<link rel="icon" href="/favicon.ico" type="image/x-icon">
	<link rel="apple-touch-icon" href="/apple-touch-icon.png">
	<style>{{style}}</style>
</head>
<body>
	<main>
		<h1>GoDrop!</h1>
		<p>Принятые файлы на компьютере</p>

		{{if .Empty}}
			<div class="card">
				<p class="error-message">Пока ничего не принято.</p>
			</div>
		{{else}}
			<div class="card">
				<p class="error-message">
					Всего файлов: {{.Total}} — {{size .Size}}
				</p>

				{{range .Groups}}
					<h3>{{.Label}}</h3>

					<ul>
						{{range .Files}}
							<li style="display: flex; gap: 10px; align-items: center;">
								<a
									href="/files/download?f={{.RelPath}}"
									style="flex: 1; min-width: 0; color: #e8e8f0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
								>{{.Name}}</a>
								<span style="color: #8888aa; font-size: 13px; white-space: nowrap;">
									{{size .Size}} · {{date .ModTime}}
								</span>
							</li>
						{{end}}
					</ul>
				{{end}}
			</div>
		{{end}}

		<a href="/"><button type="button">Загрузить файлы</button></a>

		{{if not .Empty}}
			<p class="hint">Нажмите на имя файла, чтобы скачать его на телефон.</p>
		{{end}}
	</main>

	<footer>
		Created by <strong>StrangeFragment</strong>, 2©26
	</footer>
</body>
</html>
`))

// Files показывает список принятых файлов.
func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.ErrorPage(
			w,
			http.StatusMethodNotAllowed,
			"Метод не разрешен",
			"Список файлов доступен только по GET-запросу.",
		)
		return
	}

	if !h.requireAccess(w, r) {
		return
	}

	files, err := h.Storage.List(filesPageLimit)
	if err != nil {
		log.Printf("не удалось прочитать список файлов: %v", err)

		h.ErrorPage(
			w,
			http.StatusInternalServerError,
			"Не удалось прочитать папку хранилища",
			"Проверьте, что папка хранилища существует и доступна.",
		)
		return
	}

	data := groupFiles(files)

	h.setPageHeaders(w)
	w.WriteHeader(http.StatusOK)

	if err := filesTemplate.Execute(w, data); err != nil {
		log.Printf("ошибка отрисовки страницы файлов: %v", err)
	}
}

// Download отдаёт принятый файл обратно на телефон.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	if !h.requireAccess(w, r) {
		return
	}

	relPath := r.URL.Query().Get("f")

	fullPath, err := h.Storage.Resolve(relPath)
	if err != nil {
		h.ErrorPage(
			w,
			http.StatusNotFound,
			"Файл не найден",
			"Возможно, файл уже удалён с компьютера.",
		)
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		h.ErrorPage(
			w,
			http.StatusNotFound,
			"Файл не найден",
			"Не удалось открыть файл на компьютере.",
		)
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		h.ErrorPage(
			w,
			http.StatusNotFound,
			"Файл не найден",
			"Не удалось прочитать файл на компьютере.",
		)
		return
	}

	name := filepath.Base(fullPath)

	w.Header().Set("Content-Disposition", contentDisposition(name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	http.ServeContent(w, r, name, info.ModTime(), file)
}

// contentDisposition собирает заголовок с поддержкой кириллицы в имени.
func contentDisposition(name string) string {
	fallback := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}

		return r
	}, name)

	return fmt.Sprintf(
		"attachment; filename=%q; filename*=UTF-8''%s",
		fallback,
		url.PathEscape(name),
	)
}

// groupFiles раскладывает файлы по папкам-датам.
func groupFiles(files []storage.FileInfo) filesPageData {
	data := filesPageData{
		Total: len(files),
		Empty: len(files) == 0,
	}

	index := make(map[string]int)

	for _, file := range files {
		date := strings.SplitN(file.RelPath, "/", 2)[0]

		position, ok := index[date]
		if !ok {
			position = len(data.Groups)
			index[date] = position

			data.Groups = append(data.Groups, fileGroup{
				Date:  date,
				Label: humanDate(date),
			})
		}

		data.Groups[position].Files = append(data.Groups[position].Files, file)
		data.Size += file.Size
	}

	return data
}

// humanDate переводит "2026-10-22" в "22 октября 2026".
func humanDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}

	months := [...]string{
		"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря",
	}

	return fmt.Sprintf(
		"%d %s %d",
		parsed.Day(),
		months[int(parsed.Month())-1],
		parsed.Year(),
	)
}
