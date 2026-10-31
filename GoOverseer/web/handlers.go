package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"GoOverseer/utils"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/text/encoding/charmap"
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

// ReadUtilFile пытается прочитать файл модуля сначала с локального диска utils/ (режим разработки),
// а если файл на диске отсутствует — читает его из встроенной файловой системы utils.EmbeddedFS.
func ReadUtilFile(subDir string, filename string) ([]byte, error) {
	diskPath := filepath.Join("utils", strings.ToLower(subDir), filename)
	if data, err := os.ReadFile(diskPath); err == nil {
		return data, nil
	}

	embedPath := path.Join(strings.ToLower(subDir), filename)
	return utils.EmbeddedFS.ReadFile(embedPath)
}

// getUtilSubDirs находит все папки модулей (как на диске, так и во встроенном архиве)
func getUtilSubDirs() []string {
	seen := make(map[string]bool)
	var list []string

	// 1. Проверяем локальную директорию utils на диске
	if diskEntries, err := os.ReadDir("utils"); err == nil {
		for _, e := range diskEntries {
			if e.IsDir() {
				name := strings.ToLower(e.Name())
				if !seen[name] {
					seen[name] = true
					list = append(list, name)
				}
			}
		}
	}

	// 2. Проверяем встроенную файловую систему EmbeddedFS
	if embedEntries, err := fs.ReadDir(utils.EmbeddedFS, "."); err == nil {
		for _, e := range embedEntries {
			if e.IsDir() {
				name := strings.ToLower(e.Name())
				if !seen[name] {
					seen[name] = true
					list = append(list, name)
				}
			}
		}
	}

	return list
}

func ScanUtils() ([]UtilityInfo, error) {
	var list []UtilityInfo
	subDirs := getUtilSubDirs()

	appDataDir, _ := os.UserConfigDir()

	for _, dirName := range subDirs {
		infoFileName := dirName + "_info.json"
		rawTemplate, err := ReadUtilFile(dirName, infoFileName)
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

func SetupHandlers(shutdownCtxCancel func()) *http.ServeMux {
	mux := http.NewServeMux()

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

	// Активация модуля:
	// Получает или компилирует исполняемый файл демона, копирует в %APPDATA%\GoOverseer\<ID>,
	// генерирует config.json, регистрирует в автозагрузке Windows и запускает фоновый процесс.
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
			http.Error(w, "Модуль не найден", http.StatusNotFound)
			return
		}

		// 1. Создаем рабочую папку модуля в %APPDATA%\GoOverseer\<ID>
		appDataDir, _ := os.UserConfigDir()
		liveDir := filepath.Join(appDataDir, "GoOverseer", target.ID)
		if err := os.MkdirAll(liveDir, 0755); err != nil {
			http.Error(w, "Ошибка создания папки в AppData: "+err.Error(), http.StatusInternalServerError)
			return
		}

		dstExe := filepath.Join(liveDir, target.ExeName)

		// 2. Получаем исполняемый файл демона
		exeBytes, err := ReadUtilFile(strings.ToLower(target.ID), target.ExeName)
		if err == nil {
			if writeErr := os.WriteFile(dstExe, exeBytes, 0755); writeErr != nil {
				http.Error(w, "Ошибка записи бинарника в AppData: "+writeErr.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			// Если скомпилированного .exe нет на диске (или он заблокирован антивирусом),
			// автоматически собираем демон из исходников .go прямо в целевую папку AppData!
			srcGoDisk := filepath.Join("utils", strings.ToLower(target.ID), strings.ToLower(target.ID)+"_worker.go")
			if _, statErr := os.Stat(srcGoDisk); statErr == nil {
				buildCmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -H windowsgui", "-o", dstExe, srcGoDisk)
				buildCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
				if buildErr := buildCmd.Run(); buildErr != nil {
					http.Error(w, fmt.Sprintf("Бинарник %s отсутствует, а сборка из %s завершилась с ошибкой: %v", target.ExeName, srcGoDisk, buildErr), http.StatusBadRequest)
					return
				}
			} else {
				http.Error(w, fmt.Sprintf("Исполняемый файл %s для модуля %s не найден", target.ExeName, target.ID), http.StatusBadRequest)
				return
			}
		}

		// 3. Создаем конфигурационный файл config.json
		defaultSettings := make(map[string]string)
		for _, schema := range target.UISchema {
			defaultSettings[schema.Key] = schema.Default
		}
		cfgData, _ := json.MarshalIndent(defaultSettings, "", "  ")
		_ = os.WriteFile(filepath.Join(liveDir, "config.json"), cfgData, 0644)

		// 4. Прописываем в автозагрузку Windows
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err == nil {
			_ = k.SetStringValue("GoOverseer_"+target.ID, dstExe)
			k.Close()
		}

		// 5. Запускаем процесс демона в скрытом режиме с рабочей директорией в AppData
		cmd := exec.Command(dstExe)
		cmd.Dir = liveDir
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
		if err := cmd.Start(); err != nil {
			log.Printf("Предупреждение: не удалось запустить демон на лету: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Activated successfully"))
	})

	// Деактивация модуля:
	// Принудительно гасит процесс, стирает ключ из реестра Windows и полностью зачищает папку в AppData.
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
			http.Error(w, "Модуль не найден", http.StatusNotFound)
			return
		}

		// 1. Принудительно завершаем процесс и все его дочерние процессы без мигания консоли
		killCmd := exec.Command("taskkill", "/F", "/T", "/IM", target.ExeName)
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = killCmd.Run()

		// 2. Удаляем ключ из автозагрузки Windows
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err == nil {
			_ = k.DeleteValue("GoOverseer_" + target.ID)
			k.Close()
		}

		// 3. Зачищаем все файлы и папку модуля из %APPDATA%\GoOverseer\<ID>
		appDataDir, _ := os.UserConfigDir()
		liveDir := filepath.Join(appDataDir, "GoOverseer", target.ID)

		var removeErr error
		for attempt := 0; attempt < 10; attempt++ {
			removeErr = os.RemoveAll(liveDir)
			if removeErr == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		if removeErr != nil {
			log.Printf("Предупреждение: не удалось полностью удалить директорию %s: %v", liveDir, removeErr)
			http.Error(w, "Ошибка зачистки файлов: "+removeErr.Error(), http.StatusInternalServerError)
			return
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
			http.Error(w, "Папка модуля не найдена или модуль не активен", http.StatusNotFound)
			return
		}

		formattedConfig, _ := json.MarshalIndent(newSettings, "", "  ")
		if err := os.WriteFile(liveConfigPath, formattedConfig, 0644); err != nil {
			http.Error(w, "Ошибка записи конфигурации: "+err.Error(), http.StatusInternalServerError)
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

		viewHTMLName := strings.ToLower(target.ID) + "_view.html"
		htmlBytes, err := ReadUtilFile(strings.ToLower(target.ID), viewHTMLName)
		if err != nil {
			http.Error(w, "Файл аналитики модуля не найден: "+err.Error(), http.StatusInternalServerError)
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
			"CustomHTML":  template.HTML(string(htmlBytes)),
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

		w.Header().Set("X-Log-Size-Bytes", fmt.Sprintf("%d", fileInfo.Size()))

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

		if len(validJSONLines) > 300 {
			validJSONLines = validJSONLines[len(validJSONLines)-300:]
		}

		responseJSON := "[" + strings.Join(validJSONLines, ",") + "]"
		_, _ = w.Write([]byte(responseJSON))
	})

	// Очистка логов модуля
	mux.HandleFunc("POST /api/utils/logs/clear", func(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, "Модуль не найден", http.StatusNotFound)
			return
		}

		appDataDir, _ := os.UserConfigDir()
		logFilePath := filepath.Join(appDataDir, "GoOverseer", target.ID, target.LogFile)

		_ = os.WriteFile(logFilePath, []byte(""), 0644)
		_ = os.Remove(logFilePath + ".old")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Logs cleared successfully"))
	})

	// Сканирование эфира и соседних сетей Wi-Fi
	mux.HandleFunc("GET /api/utils/wifi/scan", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		res, err := scanSurroundingWifi()
		if err != nil {
			http.Error(w, "Ошибка сканирования сетей: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data, _ := json.Marshal(res)
		_, _ = w.Write(data)
	})

	// Проверка задержки до шлюза/DNS
	mux.HandleFunc("GET /api/utils/wifi/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		ms, alive := checkPingLatency(host)
		status := "excellent"
		if !alive {
			status = "failed"
		} else if ms > 150 {
			status = "high"
		} else if ms > 50 {
			status = "moderate"
		}
		resp := map[string]interface{}{
			"host":       host,
			"alive":      alive,
			"latency_ms": ms,
			"status":     status,
		}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	})

	// Роут для скриптов аналитики (сначала с диска, если нет - из EmbeddedFS)
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
		data, err := ReadUtilFile(folderName, filename)
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

// Структура для результатов сканирования соседних сетей Wi-Fi
type ScannedBSSID struct {
	BSSID     string `json:"bssid"`
	Signal    int    `json:"signal"`
	SignalDbm int    `json:"signal_dbm"`
	Radio     string `json:"radio"`
	Channel   int    `json:"channel"`
	Band      string `json:"band"`
}

type ScannedNetwork struct {
	SSID    string         `json:"ssid"`
	Auth    string         `json:"auth"`
	Encrypt string         `json:"encrypt"`
	BSSIDs  []ScannedBSSID `json:"bssids"`
}

type ScanResult struct {
	Networks           []ScannedNetwork `json:"networks"`
	ChannelCrowd24     map[int]int      `json:"channel_crowd_24"`
	ChannelCrowd5      map[int]int      `json:"channel_crowd_5"`
	RecommendedChannel int              `json:"recommended_channel"`
	TotalBSSIDs        int              `json:"total_bssids"`
	ScanTime           string           `json:"scan_time"`
}

// Парсер вывода netsh wlan show networks mode=bssid
func scanSurroundingWifi() (ScanResult, error) {
	cmd := exec.Command("netsh", "wlan", "show", "networks", "mode=bssid")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	res := ScanResult{
		Networks:       []ScannedNetwork{},
		ChannelCrowd24: make(map[int]int),
		ChannelCrowd5:  make(map[int]int),
		ScanTime:       time.Now().Format("15:04:05"),
	}

	if err != nil {
		return res, err
	}

	// Декодируем OEM CP866 вывод Windows консоли
	var rawStr string
	if utf8.Valid(out) {
		rawStr = string(out)
	} else {
		decoder := charmap.CodePage866.NewDecoder()
		utf8Bytes, decErr := decoder.Bytes(out)
		if decErr == nil {
			rawStr = string(utf8Bytes)
		} else {
			rawStr = string(out)
		}
	}

	lines := strings.Split(rawStr, "\n")
	var currentNet *ScannedNetwork
	var currentBSSID *ScannedBSSID

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])

		if strings.HasPrefix(k, "SSID ") {
			if currentNet != nil {
				if currentBSSID != nil {
					currentNet.BSSIDs = append(currentNet.BSSIDs, *currentBSSID)
					res.TotalBSSIDs++
					currentBSSID = nil
				}
				res.Networks = append(res.Networks, *currentNet)
			}
			currentNet = &ScannedNetwork{
				SSID:   v,
				BSSIDs: []ScannedBSSID{},
			}
			continue
		}

		if currentNet == nil {
			continue
		}

		switch {
		case strings.Contains(k, "Проверка подлинности") || strings.Contains(k, "Authentication"):
			currentNet.Auth = v
		case strings.Contains(k, "Шифрование") || strings.Contains(k, "Encryption"):
			currentNet.Encrypt = v
		case strings.HasPrefix(k, "BSSID "):
			if currentBSSID != nil {
				currentNet.BSSIDs = append(currentNet.BSSIDs, *currentBSSID)
				res.TotalBSSIDs++
			}
			currentBSSID = &ScannedBSSID{BSSID: v}
		case strings.Contains(k, "Сигнал") || strings.Contains(k, "Signal"):
			if currentBSSID != nil {
				val := strings.TrimSpace(strings.TrimSuffix(v, "%"))
				pct, _ := strconv.Atoi(val)
				currentBSSID.Signal = pct
				currentBSSID.SignalDbm = (pct / 2) - 100
			}
		case strings.Contains(k, "Тип радиосети") || strings.Contains(k, "Radio type"):
			if currentBSSID != nil {
				currentBSSID.Radio = v
			}
		case strings.Contains(k, "Канал") || strings.Contains(k, "Channel"):
			if currentBSSID != nil {
				ch, _ := strconv.Atoi(strings.TrimSpace(v))
				currentBSSID.Channel = ch
				if ch >= 1 && ch <= 14 {
					currentBSSID.Band = "2.4 GHz"
					res.ChannelCrowd24[ch]++
				} else if ch >= 32 {
					currentBSSID.Band = "5 GHz"
					res.ChannelCrowd5[ch]++
				}
			}
		}
	}

	if currentNet != nil {
		if currentBSSID != nil {
			currentNet.BSSIDs = append(currentNet.BSSIDs, *currentBSSID)
			res.TotalBSSIDs++
		}
		res.Networks = append(res.Networks, *currentNet)
	}

	best24 := 1
	minCrowd := 999
	for _, ch := range []int{1, 6, 11} {
		crowd := res.ChannelCrowd24[ch]
		if crowd < minCrowd {
			minCrowd = crowd
			best24 = ch
		}
	}
	res.RecommendedChannel = best24

	return res, nil
}

// Проверка задержки до шлюза/DNS
func checkPingLatency(host string) (int, bool) {
	if host == "" {
		host = "8.8.8.8"
	}
	cmd := exec.Command("ping", "-n", "1", "-w", "1000", host)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	outStr := string(out)
	for _, token := range []string{"время=", "time=", "время<", "time<"} {
		idx := strings.Index(outStr, token)
		if idx != -1 {
			sub := outStr[idx+len(token):]
			numStr := ""
			for _, r := range sub {
				if r >= '0' && r <= '9' {
					numStr += string(r)
				} else {
					break
				}
			}
			if numStr != "" {
				ms, err := strconv.Atoi(numStr)
				if err == nil {
					return ms, true
				}
			} else if strings.HasPrefix(sub, "1") || token == "время<" || token == "time<" {
				return 1, true
			}
		}
	}

	return 0, true
}
