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
	"strconv"
	"strings"
	"time"

	"GoDrop/storage"
)

const (
	// filesPageSize — сколько файлов показывать на одной странице.
	filesPageSize = 50

	// maxFilesSearchLength — ограничивает длину поискового запроса.
	maxFilesSearchLength = 180
)

// fileGroup — файлы за один день.
type fileGroup struct {
	Date  string
	Label string
	Files []storage.FileInfo
}

// filesPageData — данные страницы "Принятые файлы".
type filesPageData struct {
	Groups       []fileGroup
	Total        int
	Size         int64
	Empty        bool
	Query        string
	Page         int
	PageCount    int
	First        int
	Last         int
	HasPrevious  bool
	HasNext      bool
	PreviousURL  string
	NextURL      string
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
		<p class="page-subtitle">Принятые файлы на компьютере</p>

		<div class="card">
			<form class="search-form" action="/files" method="get" role="search">
				<label for="fileSearch">Поиск по имени файла</label>
				<div class="search-controls">
					<input
						type="search"
						id="fileSearch"
						name="q"
						value="{{.Query}}"
						placeholder="Например, фото.jpg"
						autocomplete="off"
					>
					<button type="submit">Найти</button>
				</div>
				{{if .Query}}
					<a class="clear-search" href="/files">Сбросить поиск</a>
				{{end}}
			</form>

			{{if .Empty}}
				{{if .Query}}
					<p class="error-message">По запросу «{{.Query}}» ничего не найдено.</p>
				{{else}}
					<p class="error-message">Пока ничего не принято.</p>
				{{end}}
			{{else}}
				<p class="error-message files-summary">
					Найдено файлов: {{.Total}} — {{size .Size}}
				</p>
				<p class="hint files-range">
					Показано {{.First}}–{{.Last}} из {{.Total}} · страница {{.Page}} из {{.PageCount}}
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

				{{if gt .PageCount 1}}
					<nav class="pagination" aria-label="Страницы списка файлов">
						{{if .HasPrevious}}
							<a href="{{.PreviousURL}}" rel="prev">← Назад</a>
						{{else}}
							<span class="disabled">← Назад</span>
						{{end}}
						<span class="page-current">Страница {{.Page}} из {{.PageCount}}</span>
						{{if .HasNext}}
							<a href="{{.NextURL}}" rel="next">Дальше →</a>
						{{else}}
							<span class="disabled">Дальше →</span>
						{{end}}
					</nav>
				{{end}}
			{{end}}
		</div>

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

	files, err := h.Storage.List(0)
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

	data := buildFilesPage(
		files,
		r.URL.Query().Get("q"),
		parseFilesPage(r.URL.Query().Get("page")),
	)

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

func buildFilesPage(files []storage.FileInfo, query string, page int) filesPageData {
	query = normalizeFilesQuery(query)
	needle := strings.ToLower(query)
	filtered := make([]storage.FileInfo, 0, len(files))

	for _, file := range files {
		if needle != "" && !strings.Contains(strings.ToLower(file.Name), needle) {
			continue
		}

		filtered = append(filtered, file)
	}

	data := filesPageData{
		Total: len(filtered),
		Empty: len(filtered) == 0,
		Query: query,
		Page:  1,
	}

	if data.Empty {
		return data
	}

	for _, file := range filtered {
		data.Size += file.Size
	}

	data.PageCount = (data.Total + filesPageSize - 1) / filesPageSize
	if page < 1 {
		page = 1
	}
	if page > data.PageCount {
		page = data.PageCount
	}

	start := (page - 1) * filesPageSize
	end := start + filesPageSize
	if end > data.Total {
		end = data.Total
	}

	data.Page = page
	data.First = start + 1
	data.Last = end
	data.HasPrevious = page > 1
	data.HasNext = page < data.PageCount
	data.Groups = groupFiles(filtered[start:end])

	if data.HasPrevious {
		data.PreviousURL = filesPageURL(page-1, query)
	}
	if data.HasNext {
		data.NextURL = filesPageURL(page+1, query)
	}

	return data
}

func normalizeFilesQuery(query string) string {
	query = strings.TrimSpace(query)
	runes := []rune(query)
	if len(runes) > maxFilesSearchLength {
		query = string(runes[:maxFilesSearchLength])
	}

	return query
}

func parseFilesPage(value string) int {
	page, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || page < 1 {
		return 1
	}

	return page
}

func filesPageURL(page int, query string) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	values.Set("page", strconv.Itoa(page))

	return "/files?" + values.Encode()
}

// groupFiles раскладывает файлы по папкам-датам.
func groupFiles(files []storage.FileInfo) []fileGroup {
	groups := make([]fileGroup, 0)
	index := make(map[string]int)

	for _, file := range files {
		date := strings.SplitN(file.RelPath, "/", 2)[0]

		position, ok := index[date]
		if !ok {
			position = len(groups)
			index[date] = position

			groups = append(groups, fileGroup{
				Date:  date,
				Label: humanDate(date),
			})
		}

		groups[position].Files = append(groups[position].Files, file)
	}

	return groups
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
