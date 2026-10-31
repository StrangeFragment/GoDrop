package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

type UtilitySchema struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	Default string `json:"default"`
}

type UtilityInfo struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ExeName     string                 `json:"exe_name"`
	LogFile     string                 `json:"log_file"`
	HasCharts   bool                   `json:"has_charts"`
	ViewType    string                 `json:"view_type"`
	Active      bool                   `json:"active"`
	UISchema    []UtilitySchema        `json:"ui_schema"`
	Settings    map[string]interface{} `json:"settings"`
}

func ScanUtils() ([]UtilityInfo, error) {
	var list []UtilityInfo
	subDirs, err := os.ReadDir("utils")
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения папки utils: %v", err)
	}

	appDataDir, _ := os.UserConfigDir()

	for _, dir := range subDirs {
		if !dir.IsDir() {
			continue
		}

		folderPath := filepath.Join("utils", dir.Name())
		files, err := os.ReadDir(folderPath)
		if err != nil {
			continue
		}

		var infoFilePath string
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), "_info.json") {
				infoFilePath = filepath.Join(folderPath, f.Name())
				break
			}
		}

		if infoFilePath == "" {
			continue
		}

		rawTemplate, err := os.ReadFile(infoFilePath)
		if err != nil {
			continue
		}

		var util UtilityInfo
		if err := json.Unmarshal(rawTemplate, &util); err != nil {
			continue
		}

		liveDir := filepath.Join(appDataDir, "GoOverseer", util.ID)
		liveConfig := filepath.Join(liveDir, "config.json")
		liveExe := filepath.Join(liveDir, util.ExeName)

		_, errConfig := os.Stat(liveConfig)
		_, errExe := os.Stat(liveExe)

		util.Settings = make(map[string]interface{})

		if errConfig == nil && errExe == nil {
			util.Active = true
			rawConfig, err := os.ReadFile(liveConfig)
			if err == nil {
				_ = json.Unmarshal(rawConfig, &util.Settings)
			}
		} else {
			util.Active = false
			for _, schema := range util.UISchema {
				util.Settings[schema.Key] = schema.Default
			}
		}

		list = append(list, util)
	}

	return list, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func SetupHandlers(shutdownCtxCancel func()) *http.ServeMux {
	mux := http.NewServeMux()

	// Статический файловый сервер
	staticFS := GetStaticFS()
	fileServer := http.FileServer(http.FS(staticFS))

	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		ext := strings.ToLower(path.Ext(r.URL.Path))

		switch ext {
		case ".js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".ico":
			w.Header().Set("Content-Type", "image/x-icon")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.StripPrefix("/static/", fileServer).ServeHTTP(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		tmplFS := GetTemplatesFS()
		tmpl, err := template.ParseFS(tmplFS, "index.html")
		if err != nil {
			http.Error(w, "Ошибка шаблона", http.StatusInternalServerError)
			return
		}
		_ = tmpl.Execute(w, nil)
	})

	mux.HandleFunc("GET /api/utils", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		utilsList, err := ScanUtils()
		if err != nil {
			_, _ = w.Write([]byte("[]"))
			return
		}
		jsonResponse, _ := json.Marshal(utilsList)
		_, _ = w.Write(jsonResponse)
	})

	mux.HandleFunc("POST /api/utils/activate", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		utilsList, _ := ScanUtils()
		var target *UtilityInfo
		for i := range utilsList {
			if strings.EqualFold(utilsList[i].ID, id) {
				target = &utilsList[i]
				break
			}
		}

		if target == nil {
			http.Error(w, "Utility not found", http.StatusNotFound)
			return
		}

		appDataDir, _ := os.UserConfigDir()
		liveDir := filepath.Join(appDataDir, "GoOverseer", target.ID)
		_ = os.MkdirAll(liveDir, 0755)

		srcExe := filepath.Join("utils", strings.ToLower(target.ID), target.ExeName)
		dstExe := filepath.Join(liveDir, target.ExeName)
		if err := copyFile(srcExe, dstExe); err != nil {
			http.Error(w, "Ошибка копирования бинарника: "+err.Error(), http.StatusInternalServerError)
			return
		}

		defaultSettings := make(map[string]string)
		for _, schema := range target.UISchema {
			defaultSettings[schema.Key] = schema.Default
		}
		cfgData, _ := json.MarshalIndent(defaultSettings, "", "  ")
		_ = os.WriteFile(filepath.Join(liveDir, "config.json"), cfgData, 0644)

		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err == nil {
			_ = k.SetStringValue("GoOverseer_"+target.ID, dstExe)
			k.Close()
		}

		cmd := exec.Command(dstExe)
		if err := cmd.Start(); err != nil {
			log.Printf("Предупреждение: не удалось запустить демон на лету: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Activated successfully"))
	})

	mux.HandleFunc("POST /api/utils/deactivate", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		utilsList, _ := ScanUtils()
		var target *UtilityInfo
		for i := range utilsList {
			if strings.EqualFold(utilsList[i].ID, id) {
				target = &utilsList[i]
				break
			}
		}

		if target == nil {
			http.Error(w, "Utility not found", http.StatusNotFound)
			return
		}

		// Завершаем процесс демона
		_ = exec.Command("taskkill", "/F", "/IM", target.ExeName).Run()

		// Удаляем ключ из автозагрузки Windows
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err == nil {
			_ = k.DeleteValue("GoOverseer_" + target.ID)
			k.Close()
		}

		// Очищаем директорию с паузой для снятия файловой блокировки Windows
		appDataDir, _ := os.UserConfigDir()
		liveDir := filepath.Join(appDataDir, "GoOverseer", target.ID)

		for attempt := 0; attempt < 3; attempt++ {
			time.Sleep(150 * time.Millisecond)
			if err := os.RemoveAll(liveDir); err == nil {
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Deactivated successfully"))
	})

	mux.HandleFunc("POST /api/utils/save", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		var newSettings map[string]string
		if err := json.Unmarshal(body, &newSettings); err != nil {
			http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
			return
		}

		appDataDir, _ := os.UserConfigDir()
		liveConfigPath := filepath.Join(appDataDir, "GoOverseer", id, "config.json")

		if _, err := os.Stat(liveConfigPath); os.IsNotExist(err) {
			http.Error(w, "Utility config folder does not exist or not active", http.StatusNotFound)
			return
		}

		formattedConfig, _ := json.MarshalIndent(newSettings, "", "  ")
		if err := os.WriteFile(liveConfigPath, formattedConfig, 0644); err != nil {
			http.Error(w, "Failed to write config file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Settings saved successfully"))
	})

	mux.HandleFunc("GET /utils/view", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		utilsList, _ := ScanUtils()
		var target *UtilityInfo
		for i := range utilsList {
			if strings.EqualFold(utilsList[i].ID, id) {
				target = &utilsList[i]
				break
			}
		}

		if target == nil || !target.Active {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		customHTMLPath := filepath.Join("utils", strings.ToLower(target.ID), strings.ToLower(target.ID)+"_view.html")
		customHTML, err := os.ReadFile(customHTMLPath)
		if err != nil {
			http.Error(w, "Файл аналитики утилиты не найден: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tmplFS := GetTemplatesFS()
		baseTmpl, err := template.ParseFS(tmplFS, "utils_view.html")
		if err != nil {
			http.Error(w, "Ошибка загрузки каркаса: "+err.Error(), http.StatusInternalServerError)
			return
		}

		viewJSName := strings.ToLower(target.ID) + "_view.js"

		data := map[string]interface{}{
			"ID":          target.ID,
			"Name":        target.Name,
			"Description": target.Description,
			"CustomHTML":  template.HTML(customHTML),
			"CustomJS":    viewJSName,
		}

		_ = baseTmpl.Execute(w, data)
	})

	// Считывание логов телеметрии
	mux.HandleFunc("GET /api/utils/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		utilsList, _ := ScanUtils()
		var target *UtilityInfo
		for i := range utilsList {
			if strings.EqualFold(utilsList[i].ID, id) {
				target = &utilsList[i]
				break
			}
		}

		if target == nil || !target.Active {
			_, _ = w.Write([]byte("[]"))
			return
		}

		appDataDir, _ := os.UserConfigDir()
		logFilePath := filepath.Join(appDataDir, "GoOverseer", target.ID, target.LogFile)

		fileInfo, err := os.Stat(logFilePath)
		if err != nil {
			_, _ = w.Write([]byte("[]"))
			return
		}

		// Если файл очень большой, читаем только хвост (последние 256 КБ)
		var fileBytes []byte
		if fileInfo.Size() > 256*1024 {
			f, err := os.Open(logFilePath)
			if err == nil {
				_, _ = f.Seek(-256*1024, io.SeekEnd)
				fileBytes, _ = io.ReadAll(f)
				f.Close()
			}
		} else {
			fileBytes, _ = os.ReadFile(logFilePath)
		}

		lines := strings.Split(string(fileBytes), "\n")
		var validJSONLines []string

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && json.Valid([]byte(trimmed)) {
				validJSONLines = append(validJSONLines, trimmed)
			}
		}

		// Ограничиваем выдачу последними 500 записями
		if len(validJSONLines) > 500 {
			validJSONLines = validJSONLines[len(validJSONLines)-500:]
		}

		responseJSON := "[" + strings.Join(validJSONLines, ",") + "]"
		_, _ = w.Write([]byte(responseJSON))
	})

	// Роут для скриптов аналитики
	mux.HandleFunc("GET /static/js/utils/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) == 0 {
			http.NotFound(w, r)
			return
		}
		filename := pathParts[len(pathParts)-1]
		nameParts := strings.Split(filename, "_")
		if len(nameParts) < 2 {
			http.NotFound(w, r)
			return
		}
		folderName := strings.ToLower(nameParts[0])
		jsPath := filepath.Join("utils", folderName, filename)
		data, err := os.ReadFile(jsPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Stopping..."))
		go shutdownCtxCancel()
	})

	return mux
}
