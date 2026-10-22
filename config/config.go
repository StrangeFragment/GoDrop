package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// appDirectoryName — имя папки с настройками и логами в профиле пользователя.
	appDirectoryName = "GoDrop"

	// legacyDirectoryName — старое имя папки (наследие GoArchivist),
	// из которой переносятся настройки при первом запуске.
	legacyDirectoryName = "GoArchivist"

	configFileName    = "config.json"
	logsDirectoryName = "logs"

	// DefaultPort — порт по умолчанию.
	DefaultPort = 777

	// DefaultLogRetentionDays — сколько дней хранить файлы логов.
	DefaultLogRetentionDays = 14

	minPort = 1
	maxPort = 65535

	// maxExtensionLength — разумный предел длины расширения.
	maxExtensionLength = 20
)

// defaultExtensions — типы файлов, которые разрешены к загрузке "из коробки".
var defaultExtensions = []string{
	".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic", ".heif", ".tif", ".tiff", ".avif",
	".mp4", ".mov", ".mkv", ".avi", ".webm", ".m4v",
	".mp3", ".m4a", ".wav", ".ogg", ".flac", ".aac",
	".pdf", ".txt", ".rtf", ".md", ".csv", ".json", ".xml",
	".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt",
	".zip", ".rar", ".7z", ".tar", ".gz", ".apk", ".epub",
}

// Paths содержит пути к файлам приложения.
type Paths struct {
	ConfigDir  string
	ConfigFile string
	LogsDir    string
}

// Config — пользовательские настройки GoDrop.
type Config struct {
	// StorageDir — папка, в которую складываются загруженные файлы.
	StorageDir string `json:"storage_dir"`

	// Port — желаемый HTTP-порт.
	Port int `json:"port"`

	// StartWithWindows — запускать программу вместе с Windows.
	StartWithWindows bool `json:"start_with_windows"`

	// AllowedExtensions — расширения файлов, которые разрешено загружать.
	AllowedExtensions []string `json:"allowed_extensions"`
}

// Default возвращает настройки по умолчанию.
func Default() Config {
	return Config{
		StorageDir:        DefaultStorageDir(),
		Port:              DefaultPort,
		StartWithWindows:  false,
		AllowedExtensions: append([]string(nil), defaultExtensions...),
	}
}

// DefaultStorageDir подбирает папку для файлов: "Загрузки\GoDrop", если папка
// загрузок существует, иначе "<профиль>\GoDrop".
func DefaultStorageDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "GoDrop"
	}

	downloads := filepath.Join(home, "Downloads")

	if info, statErr := os.Stat(downloads); statErr == nil && info.IsDir() {
		return filepath.Join(downloads, appDirectoryName)
	}

	return filepath.Join(home, appDirectoryName)
}

// UserPaths возвращает пути приложения и создаёт нужные папки.
func UserPaths() (Paths, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf(
			"не удалось определить папку настроек пользователя: %w",
			err,
		)
	}

	configDir := filepath.Join(configRoot, appDirectoryName)

	paths := Paths{
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, configFileName),
		LogsDir:    filepath.Join(configDir, logsDirectoryName),
	}

	migrateLegacy(filepath.Join(configRoot, legacyDirectoryName), configDir)

	for _, dir := range []string{paths.ConfigDir, paths.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Paths{}, fmt.Errorf("не удалось создать папку %q: %w", dir, err)
		}
	}

	return paths, nil
}

// migrateLegacy переносит config.json из старой папки приложения,
// чтобы пользователь не потерял настройки после переименования.
func migrateLegacy(oldDir, newDir string) {
	oldConfig := filepath.Join(oldDir, configFileName)
	newConfig := filepath.Join(newDir, configFileName)

	if _, err := os.Stat(oldConfig); err != nil {
		return
	}

	if _, err := os.Stat(newConfig); err == nil {
		return
	}

	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return
	}

	data, err := os.ReadFile(oldConfig)
	if err != nil {
		return
	}

	_ = os.WriteFile(newConfig, data, 0o644)
}

// LogPath возвращает путь к файлу лога текущего дня.
func LogPath() (string, error) {
	paths, err := UserPaths()
	if err != nil {
		return "", err
	}

	name := "godrop-" + time.Now().Format("2006-01-02") + ".log"

	return filepath.Join(paths.LogsDir, name), nil
}

// PruneOldLogs удаляет файлы логов старше указанного количества дней.
// Ошибки игнорируются: это обслуживающая операция.
func PruneOldLogs(days int) {
	if days <= 0 {
		days = DefaultLogRetentionDays
	}

	paths, err := UserPaths()
	if err != nil {
		return
	}

	entries, err := os.ReadDir(paths.LogsDir)
	if err != nil {
		return
	}

	deadline := time.Now().AddDate(0, 0, -days)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil || info.ModTime().After(deadline) {
			continue
		}

		_ = os.Remove(filepath.Join(paths.LogsDir, entry.Name()))
	}
}

// Load читает настройки. Если файла нет — создаёт его со значениями
// по умолчанию. Некорректный JSON не роняет программу: возвращаются
// настройки по умолчанию и ошибка, которую вызывающий код только логирует.
func Load() (Config, error) {
	paths, err := UserPaths()
	if err != nil {
		return Default(), err
	}

	cfg, loadErr := LoadFile(paths.ConfigFile)
	if loadErr != nil {
		if errors.Is(loadErr, os.ErrNotExist) {
			cfg = Default()
			if saveErr := SaveTo(paths.ConfigFile, cfg); saveErr != nil {
				return cfg, fmt.Errorf("не удалось создать config.json: %w", saveErr)
			}

			return cfg, nil
		}

		return cfg, loadErr
	}

	return cfg, nil
}

// LoadFile читает настройки из конкретного файла.
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Default(), err
	}

	cfg := Config{}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("ошибка разбора %s: %w", filepath.Base(path), err)
	}

	// Совместимость со старой версией файла, где поля сериализовались
	// без тегов (StorageDir, Port, ...). Читаем такое же содержимое
	// во вторую структуру и подставляем недостающие поля.
	var legacy legacyConfig
	if legacyErr := json.Unmarshal(data, &legacy); legacyErr == nil {
		if cfg.StorageDir == "" {
			cfg.StorageDir = legacy.StorageDir
		}
		if cfg.Port == 0 {
			cfg.Port = legacy.Port
		}
		if len(cfg.AllowedExtensions) == 0 {
			cfg.AllowedExtensions = legacy.AllowedExtensions
		}
	}

	cfg.Normalize()

	return cfg, nil
}

type legacyConfig struct {
	StorageDir        string
	Port              int
	StartWithWindows  bool
	AllowedExtensions []string
}

// Save записывает настройки в стандартный файл.
func Save(cfg Config) error {
	paths, err := UserPaths()
	if err != nil {
		return err
	}

	return SaveTo(paths.ConfigFile, cfg)
}

// SaveTo записывает настройки в указанный файл атомарно
// (через временный файл), чтобы не получить "половину" config.json.
func SaveTo(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("путь к файлу настроек не задан")
	}

	cfg.Normalize()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("не удалось подготовить config.json: %w", err)
	}

	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("не удалось создать папку настроек: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("не удалось сохранить config.json: %w", err)
	}

	tempPath := temp.Name()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)

		return fmt.Errorf("не удалось сохранить config.json: %w", err)
	}

	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("не удалось сохранить config.json: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("не удалось сохранить config.json: %w", err)
	}

	return nil
}

// Normalize приводит настройки к корректному виду:
// подставляет значения по умолчанию и чистит список расширений.
func (c *Config) Normalize() {
	defaults := Default()

	c.StorageDir = strings.TrimSpace(c.StorageDir)
	if c.StorageDir == "" {
		c.StorageDir = defaults.StorageDir
	} else {
		c.StorageDir = filepath.Clean(c.StorageDir)
	}

	if c.Port < minPort || c.Port > maxPort {
		c.Port = defaults.Port
	}

	c.AllowedExtensions = NormalizeExtensions(c.AllowedExtensions)
	if len(c.AllowedExtensions) == 0 {
		c.AllowedExtensions = defaults.AllowedExtensions
	}
}

// Validate проверяет настройки перед применением.
func (c Config) Validate() error {
	if strings.TrimSpace(c.StorageDir) == "" {
		return errors.New("папка хранилища не может быть пустой")
	}

	if c.Port < minPort || c.Port > maxPort {
		return fmt.Errorf("порт должен быть числом от %d до %d", minPort, maxPort)
	}

	if len(c.AllowedExtensions) == 0 {
		return errors.New("укажите хотя бы одно разрешённое расширение")
	}

	return nil
}

// ParsePort разбирает строку с номером порта.
func ParsePort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < minPort || port > maxPort {
		return 0, fmt.Errorf("порт должен быть числом от %d до %d", minPort, maxPort)
	}

	return port, nil
}

// ParseExtensions разбирает строку вида ".jpg, png; .pdf" в список расширений.
func ParseExtensions(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})

	return NormalizeExtensions(parts)
}

// NormalizeExtensions приводит расширения к виду ".ext": нижний регистр,
// без пробелов, без дублей и без мусора.
func NormalizeExtensions(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		ext := NormalizeExtension(value)
		if ext == "" {
			continue
		}

		if _, ok := seen[ext]; ok {
			continue
		}

		seen[ext] = struct{}{}
		result = append(result, ext)
	}

	sort.Strings(result)

	return result
}

// NormalizeExtension приводит одно расширение к виду ".ext".
// Возвращает пустую строку, если расширение некорректно.
func NormalizeExtension(value string) string {
	ext := strings.ToLower(strings.TrimSpace(value))
	ext = strings.TrimPrefix(ext, "*")

	if ext == "" || ext == "." {
		return ""
	}

	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	// Допустимый вид: ".ext" или ".tar.gz" — точки только как разделители,
	// никаких "..", точек в конце и посторонних символов.
	parts := strings.Split(ext[1:], ".")

	if len(parts) == 0 {
		return ""
	}

	for _, part := range parts {
		if part == "" {
			return ""
		}

		for _, r := range part {
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !ok {
				return ""
			}
		}
	}

	if len(ext) > maxExtensionLength {
		return ""
	}

	return ext
}

// ExtensionsString возвращает расширения одной строкой для полей ввода.
func (c Config) ExtensionsString() string {
	return strings.Join(c.AllowedExtensions, ", ")
}

// AcceptAttribute возвращает значение для HTML-атрибута accept,
// чтобы на телефоне в списке файлов были только разрешённые типы.
func (c Config) AcceptAttribute() string {
	return strings.Join(c.AllowedExtensions, ",")
}
