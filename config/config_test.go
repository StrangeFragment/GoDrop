package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeExtension(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"jpg":      ".jpg",
		".JPG":     ".jpg",
		"  .Png  ": ".png",
		"*.pdf":    ".pdf",
		".tar.gz":  ".tar.gz",
		"":         "",
		".":        "",
		"..":       "",
		".j pg":    "",
		".jp*g":    "",
	}

	for input, want := range cases {
		if got := NormalizeExtension(input); got != want {
			t.Errorf("NormalizeExtension(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseExtensions(t *testing.T) {
	t.Parallel()

	got := ParseExtensions(" JPG,.png; png , pdf\nmp4\tnonsense*")

	want := []string{".jpg", ".mp4", ".pdf", ".png"}

	if len(got) != len(want) {
		t.Fatalf("ParseExtensions() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseExtensions() = %v, want %v", got, want)
		}
	}
}

func TestNormalizeFillsDefaults(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	cfg.Normalize()

	if cfg.StorageDir == "" {
		t.Error("StorageDir должен быть заполнен значением по умолчанию")
	}

	if cfg.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, DefaultPort)
	}

	if len(cfg.AllowedExtensions) == 0 {
		t.Error("AllowedExtensions должен быть непустым")
	}
}

func TestNormalizeKeepsValidPort(t *testing.T) {
	t.Parallel()

	cfg := Config{Port: 8080}
	cfg.Normalize()

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}

	cfg = Config{Port: 70000}
	cfg.Normalize()

	if cfg.Port != DefaultPort {
		t.Errorf("некорректный порт должен заменяться, получено %d", cfg.Port)
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := Config{
		StorageDir:        filepath.Join(dir, "files"),
		Port:              888,
		StartWithWindows:  true,
		AllowedExtensions: []string{".JPG", "png"},
	}

	if err := SaveTo(path, original); err != nil {
		t.Fatalf("SaveTo() error: %v", err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}

	if loaded.Port != 888 {
		t.Errorf("Port = %d, want 888", loaded.Port)
	}

	if !loaded.StartWithWindows {
		t.Error("StartWithWindows должен сохраняться")
	}

	if len(loaded.AllowedExtensions) != 2 {
		t.Fatalf("AllowedExtensions = %v", loaded.AllowedExtensions)
	}
}

func TestLoadFileLegacyFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Старый формат: поля без json-тегов, как писалось предыдущей версией.
	legacy := `{
  "StorageDir": "C:\\Users\\user\\Downloads",
  "Port": 777,
  "StartWithWindows": false,
  "AllowedExtensions": [".jpg", ".png"]
}`

	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("не удалось записать файл: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}

	if cfg.StorageDir != `C:\Users\user\Downloads` {
		t.Errorf("StorageDir = %q", cfg.StorageDir)
	}

	if len(cfg.AllowedExtensions) != 2 {
		t.Errorf("AllowedExtensions = %v", cfg.AllowedExtensions)
	}
}

func TestLoadFileBrokenJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatalf("не удалось записать файл: %v", err)
	}

	cfg, err := LoadFile(path)
	if err == nil {
		t.Fatal("ожидалась ошибка разбора")
	}

	// Даже при ошибке возвращаются рабочие значения по умолчанию.
	if err := cfg.Validate(); err != nil {
		t.Errorf("настройки по умолчанию должны быть валидны: %v", err)
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"777", " 8080 "} {
		if _, err := ParsePort(value); err != nil {
			t.Errorf("ParsePort(%q) unexpected error: %v", value, err)
		}
	}

	for _, value := range []string{"", "abc", "0", "-1", "65536"} {
		if _, err := ParsePort(value); err == nil {
			t.Errorf("ParsePort(%q) должна завершаться ошибкой", value)
		}
	}
}

func TestNormalizeAccessCode(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"":            "",
		"  ":          "",
		"123456":      "123456",
		" abcd-_12 ":  "abcd-_12",
		"ABCDEFGHIJK": "ABCDEFGHIJK",
	}

	for input, want := range valid {
		got, err := NormalizeAccessCode(input)
		if err != nil {
			t.Errorf("NormalizeAccessCode(%q) error: %v", input, err)
			continue
		}

		if got != want {
			t.Errorf("NormalizeAccessCode(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{"123", "абвгд", "код 123", "1234567890123456789012345678901234", "12 34"}

	for _, input := range invalid {
		if _, err := NormalizeAccessCode(input); err == nil {
			t.Errorf("NormalizeAccessCode(%q) должна возвращать ошибку", input)
		}
	}
}

func TestGenerateAccessCode(t *testing.T) {
	t.Parallel()

	code, err := GenerateAccessCode(6)
	if err != nil {
		t.Fatalf("GenerateAccessCode() error: %v", err)
	}

	if len(code) != 6 {
		t.Fatalf("длина кода = %d, want 6", len(code))
	}

	if _, err := NormalizeAccessCode(code); err != nil {
		t.Errorf("сгенерированный код не проходит проверку: %v", err)
	}

	second, err := GenerateAccessCode(6)
	if err != nil {
		t.Fatalf("GenerateAccessCode() error: %v", err)
	}

	if code == second {
		t.Log("коды совпали — статистически маловероятно, но допустимо")
	}
}

func TestValidateRejectsBrokenAccessCode(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.AccessCode = "код"

	if err := cfg.Validate(); err == nil {
		t.Fatal("ожидалась ошибка для нелатинского кода доступа")
	}
}
