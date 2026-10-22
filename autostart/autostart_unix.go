//go:build linux || freebsd || openbsd || netbsd

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func enabled() (bool, error) {
	path, err := autostartFile()
	if err != nil {
		return false, err
	}

	_, err = os.Stat(path)

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

func enableAutostart() error {
	executable, err := ExecutablePath()
	if err != nil {
		return err
	}

	path, err := autostartFile()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("не удалось создать папку автозапуска: %w", err)
	}

	if err := os.WriteFile(path, []byte(desktopEntry(executable)), 0o644); err != nil {
		return fmt.Errorf("не удалось включить автозапуск: %w", err)
	}

	return nil
}

func disableAutostart() error {
	path, err := autostartFile()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("не удалось отключить автозапуск: %w", err)
	}

	return nil
}
