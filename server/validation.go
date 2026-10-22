package server

import (
	"path/filepath"
	"strings"
)

func isAllowed(filename string, allowed []string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	for _, item := range allowed {
		if strings.ToLower(strings.TrimSpace(item)) == ext {
			return true
		}
	}

	return false
}
