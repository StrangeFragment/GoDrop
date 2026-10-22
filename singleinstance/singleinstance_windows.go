//go:build windows

package singleinstance

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func acquire(name string) (*Lock, bool, error) {
	// "Local\" — мьютекс в рамках текущей сессии пользователя.
	// Так программа не конфликтует сама с собой при переключении
	// между пользователями Windows.
	fullName := `Local\` + name

	namePtr, err := windows.UTF16PtrFromString(fullName)
	if err != nil {
		return nil, false, fmt.Errorf("некорректное имя блокировки: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, false, fmt.Errorf("не удалось создать мьютекс: %w", err)
	}

	if handle == 0 {
		return nil, false, fmt.Errorf("не удалось создать мьютекс: пустой дескриптор")
	}

	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Дескриптор нужно закрыть: мьютекс уже существовал.
		_ = windows.CloseHandle(handle)

		return nil, false, nil
	}

	return &Lock{
		release: func() {
			_ = windows.CloseHandle(handle)
		},
	}, true, nil
}
