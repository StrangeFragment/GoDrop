// Package autostart включает и выключает запуск GoDrop вместе с системой.
//
// Windows: раздел реестра HKCU\...\CurrentVersion\Run — не требует прав
// администратора.
// Linux: файл .desktop в ~/.config/autostart.
// macOS: не поддерживается (нужен LaunchAgent), функция возвращает ошибку.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// appName — имя значения в реестре и имя файла .desktop.
	appName = "GoDrop"

	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// ExecutablePath возвращает путь к текущему исполняемому файлу.
func ExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("не удалось определить путь к программе: %w", err)
	}

	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolved
	}

	return path, nil
}

// Enabled сообщает, включён ли автозапуск.
func Enabled() (bool, error) {
	return enabled()
}

// Set включает или выключает автозапуск.
func Set(enable bool) error {
	if enable {
		return enableAutostart()
	}

	return disableAutostart()
}

// errUnsupported — платформа без поддержки автозапуска.
var errUnsupported = errors.New("автозапуск не поддерживается на этой платформе")

// quoteForRunKey экранирует путь для значения реестра.
func quoteForRunKey(path string) string {
	if strings.HasPrefix(path, `"`) {
		return path
	}

	return `"` + path + `"`
}

// desktopEntry — содержимое файла автозапуска для Linux.
func desktopEntry(executable string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=GoDrop
Comment=Передача файлов с телефона на компьютер
Exec=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, executable)
}

// autostartFile — путь к файлу автозапуска (Linux).
func autostartFile() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("не удалось определить папку настроек: %w", err)
	}

	return filepath.Join(configDir, "autostart", "godrop.desktop"), nil
}
