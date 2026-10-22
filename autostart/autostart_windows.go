//go:build windows

package autostart

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

func openKey(access uint32) (registry.Key, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, access)
	if err != nil {
		return 0, fmt.Errorf("не удалось открыть раздел автозапуска: %w", err)
	}

	return key, nil
}

func enabled() (bool, error) {
	key, err := openKey(registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	defer func() { _ = key.Close() }()

	value, _, err := key.GetStringValue(appName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("не удалось прочитать параметр автозапуска: %w", err)
	}

	return value != "", nil
}

func enableAutostart() error {
	executable, err := ExecutablePath()
	if err != nil {
		return err
	}

	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		runKey,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("не удалось открыть раздел автозапуска: %w", err)
	}
	defer func() { _ = key.Close() }()

	if err := key.SetStringValue(appName, quoteForRunKey(executable)); err != nil {
		return fmt.Errorf("не удалось включить автозапуск: %w", err)
	}

	return nil
}

func disableAutostart() error {
	key, err := openKey(registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}

		return err
	}
	defer func() { _ = key.Close() }()

	if err := key.DeleteValue(appName); err != nil &&
		!errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("не удалось отключить автозапуск: %w", err)
	}

	return nil
}
