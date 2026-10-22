package tray

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/gogpu/systray"

	"GoDrop/assets"
	"GoDrop/config"
	"GoDrop/settings"
)

type Callbacks struct {
	URL        string
	StorageDir string
	Config     config.Config

	OnSettingsSaved func(config.Config) (string, error)
	OnExit          func()
}

func Run(callbacks Callbacks) {
	tray := systray.New()
	var exitOnce sync.Once

	exit := func() {
		exitOnce.Do(func() {
			if callbacks.OnExit != nil {
				callbacks.OnExit()
			}
			tray.Remove()
		})
	}

	menu := systray.NewMenu()

	menu.Add("GoDrop!", func() {
		if err := settings.ShowQR(callbacks.URL); err != nil {
			fmt.Println("Ошибка открытия QR-кода:", err)
		}
	})

	menu.Add("Хранилище", func() {
		openFolder(callbacks.StorageDir)
	})

	menu.Add("Настройки", func() {
		nextConfig, ok := showSettings(callbacks.Config)
		if !ok || callbacks.OnSettingsSaved == nil {
			return
		}

		nextURL, err := callbacks.OnSettingsSaved(nextConfig)
		if err != nil {
			fmt.Println("Ошибка сохранения настроек:", err)
			return
		}

		callbacks.Config = nextConfig
		callbacks.StorageDir = nextConfig.StorageDir
		callbacks.URL = nextURL

		if err := settings.UpdateQR(nextURL); err != nil {
			fmt.Println("Ошибка обновления QR-кода:", err)
		}
	})

	menu.AddSeparator()

	menu.Add("Выход", func() {
		exit()
		os.Exit(0)
	})

	tray.SetIcon(assets.GoDrop64).
		SetTooltip("GoDrop — передача файлов").
		SetMenu(menu)

	// Левый клик ничего не делает; правый открывает контекстное меню.
	tray.OnRightClick(func() {})

	tray.Show()

	if err := tray.Run(); err != nil {
		fmt.Println("Ошибка системного трея:", err)
	}

	exit()
}

func showSettings(current config.Config) (config.Config, bool) {
	return settings.Show(current)
}

func openFolder(path string) {
	_ = os.MkdirAll(path, 0755)

	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer.exe", path).Start()
	}
}

func Address(ip string, port int) string {
	return fmt.Sprintf("http://%s:%d", ip, port)
}
