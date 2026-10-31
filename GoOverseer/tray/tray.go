package tray

import (
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/gogpu/systray"
)

// InitTray настраивает трей и возвращает объект наружу для запуска в главном потоке
func InitTray(serverURL string, iconBytes []byte, onExitRequested func()) *systray.SystemTray {
	tray := systray.New()

	// Создаем контекстное меню (правая кнопка мыши)
	menu := systray.NewMenu()
	menu.Add("Открыть панель", func() {
		// ИСПРАВЛЕНО: Изменили на заглавную букву
		OpenBrowserInWindows(serverURL)
	})
	menu.AddSeparator()
	menu.Add("Выйти", func() {
		tray.Remove() // Выводит программу из блокировки t.Run()
		onExitRequested()
	})

	// Конфигурируем трей
	tray.SetIcon(iconBytes).SetTooltip("GoOverseer Контроллер").SetMenu(menu)

	// Привязываем клик левой кнопкой мыши
	tray.OnClick(func() {
		// ИСПРАВЛЕНО: Изменили на заглавную букву
		OpenBrowserInWindows(serverURL)
	})

	tray.Show()

	return tray
}

// OpenBrowserInWindows открывает ссылку через стандартную оболочку cmd.exe в Windows
func OpenBrowserInWindows(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = "C:\\Windows"
		}
		cmdPath := systemRoot + "\\System32\\cmd.exe"
		err = exec.Command(cmdPath, "/c", "start", url).Start()
	default:
		log.Printf("Открытие браузера не поддерживается на ОС: %s", runtime.GOOS)
	}
	if err != nil {
		log.Printf("Не удалось запустить браузер: %v", err)
	}
}
