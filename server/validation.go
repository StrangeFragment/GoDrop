package server

import (
	"path/filepath"
	"strings"

	"GoDrop/config"
)

// isAllowed проверяет, что расширение файла входит в список разрешённых.
func isAllowed(filename string, allowed []string) bool {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return false
	}

	ext := filepath.Ext(filename)
	if ext == "" {
		return false
	}

	// Приводим оба значения к виду ".ext", чтобы прощались
	// "* .JPG", "jpeg" и прочие варианты записи.
	ext = config.NormalizeExtension(ext)
	if ext == "" {
		return false
	}

	for _, item := range allowed {
		if config.NormalizeExtension(item) == ext {
			return true
		}
	}

	return false
}
