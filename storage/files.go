package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// tempSuffix — суффикс временного файла, который переименовывается
	// в итоговое имя только после успешного сохранения.
	tempSuffix = ".godrop-tmp"

	// maxNameLength — ограничение длины имени файла в байтах.
	maxNameLength = 180

	// maxPathLength — ограничение длины полного пути (Windows: 260 символов).
	maxPathLength = 240
)

// запрещённые в Windows символы имени файла
const forbiddenNameChars = `<>:"/\|?*`

// зарезервированные имена Windows
var reservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// Storage отвечает за сохранение принятых файлов на диск.
type Storage struct {
	BaseDir string
}

// New создаёт хранилище в указанной папке.
func New(baseDir string) *Storage {
	return &Storage{
		BaseDir: baseDir,
	}
}

// TodayDir создаёт (при необходимости) и возвращает папку текущей даты.
func (s *Storage) TodayDir() (string, error) {
	if strings.TrimSpace(s.BaseDir) == "" {
		return "", fmt.Errorf("папка хранилища не задана")
	}

	date := time.Now().Format("2006-01-02")
	dir := filepath.Join(s.BaseDir, date)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("не удалось создать папку %q: %w", dir, err)
	}

	return dir, nil
}

// SaveStream сохраняет данные из src в хранилище и возвращает итоговое имя файла.
func (s *Storage) SaveStream(filename string, src io.Reader) (string, error) {
	dir, err := s.TodayDir()
	if err != nil {
		return "", err
	}

	name, err := SanitizeName(filename)
	if err != nil {
		return "", err
	}

	finalPath, err := reserveUnique(dir, name)
	if err != nil {
		return "", err
	}

	tempPath := finalPath + tempSuffix

	file, err := os.OpenFile(
		tempPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		_ = os.Remove(finalPath)
		return "", fmt.Errorf("не удалось создать файл: %w", err)
	}

	_, copyErr := io.Copy(file, src)
	closeErr := file.Close()

	if copyErr != nil {
		_ = os.Remove(tempPath)
		_ = os.Remove(finalPath)

		return "", fmt.Errorf("не удалось записать файл: %w", copyErr)
	}

	if closeErr != nil {
		_ = os.Remove(tempPath)
		_ = os.Remove(finalPath)

		return "", fmt.Errorf("не удалось записать файл: %w", closeErr)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		_ = os.Remove(finalPath)

		return "", fmt.Errorf("не удалось сохранить файл: %w", err)
	}

	return filepath.Base(finalPath), nil
}

// SanitizeName очищает имя файла от опасных символов и служебных имён.
func SanitizeName(filename string) (string, error) {
	// Имя может прийти как с "/", так и с "\" (браузер на Windows
	// иногда присылает полный путь), поэтому приводим к одному виду.
	filename = strings.ReplaceAll(strings.TrimSpace(filename), `\`, "/")
	filename = strings.TrimRight(filename, "/")
	name := path.Base(filename)

	// Отбрасываем управляющие символы и непечатный мусор.
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}

		if strings.ContainsRune(forbiddenNameChars, r) {
			return '_'
		}

		return r
	}, name)

	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ". ")

	if name == "" || name == "." || name == ".." || name == "/" {
		return "", fmt.Errorf("некорректное имя файла")
	}

	// Windows не умеет в зарезервированные имена (CON, NUL, COM1, ...).
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if _, ok := reservedNames[strings.ToLower(base)]; ok {
		name = "_" + name
	}

	name = truncateName(name, maxNameLength)

	if name == "" {
		return "", fmt.Errorf("некорректное имя файла")
	}

	return name, nil
}

// truncateName обрезает имя, сохраняя расширение и не разрывая UTF-8.
func truncateName(name string, limit int) string {
	if len(name) <= limit {
		return name
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	available := limit - len(ext)
	if available < 1 {
		ext = ""
		base = name
		available = limit
	}

	for len(base) > available && len(base) > 0 {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}

	return strings.TrimRight(base, ". ") + ext
}

// reserveUnique атомарно занимает свободное имя в папке и возвращает путь.
//
// Файл создаётся с флагом O_EXCL: если два телефона одновременно пришлют
// файлы с одинаковыми именами, второй получит следующее имя "file (1).ext"
// вместо того, чтобы затереть файл первого.
func reserveUnique(dir, filename string) (string, error) {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	for i := 0; i < 10000; i++ {
		attempt := filename

		if i > 0 {
			attempt = fmt.Sprintf("%s (%d)%s", name, i, ext)
		}

		// Слишком длинный путь тоже не годится — сокращаем основу имени.
		if len(filepath.Join(dir, attempt)) > maxPathLength {
			attempt = fmt.Sprintf("%s(%d)%s", truncateName(name, 60), i, ext)
		}

		candidate := filepath.Join(dir, attempt)

		file, err := os.OpenFile(
			candidate,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			0o644,
		)
		if err == nil {
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(candidate)
				return "", closeErr
			}

			return candidate, nil
		}

		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("не удалось создать файл: %w", err)
		}
	}

	return "", fmt.Errorf("не удалось подобрать свободное имя файла")
}
