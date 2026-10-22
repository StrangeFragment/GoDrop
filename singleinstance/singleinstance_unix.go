//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquire(name string) (*Lock, bool, error) {
	path := filepath.Join(os.TempDir(), name+".lock")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("не удалось открыть файл блокировки: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()

		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}

		return nil, false, fmt.Errorf("не удалось получить блокировку: %w", err)
	}

	return &Lock{
		release: func() {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
		},
	}, true, nil
}
