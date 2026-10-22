package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Storage struct {
	BaseDir string
}

func New(baseDir string) *Storage {
	return &Storage{
		BaseDir: baseDir,
	}
}

func (s *Storage) TodayDir() (string, error) {
	date := time.Now().Format("2006-01-02")
	dir := filepath.Join(s.BaseDir, date)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	return dir, nil
}

func (s *Storage) SaveStream(filename string, src io.Reader) (string, error) {
	dir, err := s.TodayDir()
	if err != nil {
		return "", err
	}

	filename = filepath.Base(filename)
	filename = strings.TrimSpace(filename)

	if filename == "" || filename == "." || filename == ".." {
		return "", fmt.Errorf("некорректное имя файла")
	}

	finalPath := uniquePath(dir, filename)
	tempPath := finalPath + ".godrop-tmp"

	file, err := os.OpenFile(
		tempPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)
	if err != nil {
		return "", err
	}

	_, copyErr := io.Copy(file, src)
	closeErr := file.Close()

	if copyErr != nil {
		_ = os.Remove(tempPath)
		return "", copyErr
	}

	if closeErr != nil {
		_ = os.Remove(tempPath)
		return "", closeErr
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	return filepath.Base(finalPath), nil
}

func uniquePath(dir, filename string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	candidate := filepath.Join(dir, filename)

	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}

		candidate = filepath.Join(
			dir,
			fmt.Sprintf("%s (%d)%s", name, i, ext),
		)
	}
}
