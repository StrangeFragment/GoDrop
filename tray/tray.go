package tray

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gogpu/systray"

	"GoDrop/assets"
	"GoDrop/config"
	"GoDrop/settings"
)

// Links — адреса, которые показывает программа.
type Links struct {
	// URL — адрес для телефона (с кодом доступа, если он задан).
	URL string

	// BaseURL — адрес без кода доступа.
	BaseURL string

	// Code — код доступа (пустая строка, если не задан).
	Code string
}

// Callbacks — обработчики меню трея.
type Callbacks struct {
	Links
	StorageDir string
	Config     config.Config

	OnSettingsSaved func(config.Config) (Links, error)
	OnExit          func()
}

// windowKind — тип вспомогательного окна.
type windowKind string

const (
	windowQR       windowKind = "qr"
	windowSettings windowKind = "settings"

	// windowCloseTimeout — сколько ждать закрытия окон при выходе.
	windowCloseTimeout = 3 * time.Second
)

var (
	notifyMu sync.Mutex
	notifyFn func(string)
)

// Notify показывает уведомление рядом со значком в трее.
// Функция безопасна для вызова из любой горутины, в том числе
// из HTTP-обработчика после сохранения файлов.
func Notify(message string) {
	notifyMu.Lock()
	fn := notifyFn
	notifyMu.Unlock()

	if fn != nil {
		fn(message)
	}
}

func setNotifier(fn func(string)) {
	notifyMu.Lock()
	notifyFn = fn
	notifyMu.Unlock()
}

// Run запускает значок в системном трее и блокируется до выхода.
func Run(callbacks Callbacks) {
	tray := systray.New()

	var (
		exitOnce sync.Once
		stateMu  sync.Mutex // защищает поля callbacks, изменяемые из горутин
		windows  = newWindowTracker()
	)

	// currentCallbacks возвращает копию данных для обработчика.
	currentCallbacks := func() Callbacks {
		stateMu.Lock()
		defer stateMu.Unlock()

		return callbacks
	}

	exit := func() {
		exitOnce.Do(func() {
			if callbacks.OnExit != nil {
				callbacks.OnExit()
			}

			tray.Remove()
		})
	}

	// notify показывает уведомление и пишет строку в консоль:
	// на Windows всплывает подсказка, в остальных системах — хотя бы лог.
	notify := func(message string) {
		fmt.Println(message)
		log.Println(message)

		// Показ уведомления — вызов Win32/D-Bus; сериализуем его,
		// чтобы одновременные загрузки не дёргали трей параллельно.
		notifyMu.Lock()
		defer notifyMu.Unlock()

		tray.ShowNotification("GoDrop!", message)
	}

	setNotifier(notify)
	defer setNotifier(nil)

	// runWindow запускает окно в отдельной горутине и не даёт открыть
	// его повторно, пока предыдущее не закрыто.
	//
	// Важно: обработчики меню вызываются в потоке цикла сообщений ОС,
	// поэтому блокировать их (как раньше) нельзя — иначе трей перестаёт
	// реагировать на клики, пока открыто окно.
	runWindow := func(kind windowKind, alreadyOpenMessage string, action func()) {
		if !windows.tryOpen(kind) {
			notify(alreadyOpenMessage)
			return
		}

		go func() {
			defer windows.close(kind)
			action()
		}()
	}

	menu := systray.NewMenu()

	showQR := func() {
		runWindow(windowQR, "Окно с QR-кодом уже открыто", func() {
			cb := currentCallbacks()

			if err := settings.ShowQR(cb.URL, cb.Code); err != nil {
				notify("Ошибка открытия QR-кода: " + err.Error())
			}
		})
	}

	menu.Add("GoDrop!", showQR)

	menu.Add("Принятые файлы", func() {
		go func() {
			if err := openURL(currentCallbacks().BaseURL + "/files"); err != nil {
				notify("Не удалось открыть браузер: " + err.Error())
			}
		}()
	})

	menu.Add("Хранилище", func() {
		if err := openFolder(currentCallbacks().StorageDir); err != nil {
			notify("Не удалось открыть папку хранилища: " + err.Error())
		}
	})

	menu.Add("Настройки", func() {
		runWindow(windowSettings, "Окно настроек уже открыто", func() {
			nextConfig, ok, err := settings.Show(currentCallbacks().Config)
			if err != nil {
				notify("Ошибка окна настроек: " + err.Error())
				return
			}

			if !ok {
				return
			}

			cb := currentCallbacks()
			if cb.OnSettingsSaved == nil {
				return
			}

			links, err := cb.OnSettingsSaved(nextConfig)
			if err != nil {
				notify("Ошибка сохранения настроек: " + err.Error())
				return
			}

			stateMu.Lock()
			callbacks.Config = nextConfig
			callbacks.StorageDir = nextConfig.StorageDir
			callbacks.Links = links
			stateMu.Unlock()

			if err := settings.UpdateQR(links.URL, links.Code); err != nil {
				log.Printf("не удалось обновить QR-код: %v", err)
			}
		})
	})

	menu.AddSeparator()

	menu.Add("Выход", func() {
		// Remove() останавливает цикл сообщений, tray.Run() возвращается,
		// и main корректно завершает программу (лог закрывается в defer).
		exit()
	})

	tray.SetIcon(assets.GoDrop64).
		SetAppName("GoDrop").
		SetTooltip("GoDrop — передача файлов").
		SetMenu(menu)

	// Левый клик открывает окно с QR-кодом — так удобнее, чем лезть в меню.
	tray.OnClick(showQR)

	tray.Show()

	if err := tray.Run(); err != nil {
		fmt.Println("Ошибка системного трея:", err)
		log.Printf("ошибка системного трея: %v", err)
	}

	// Даём открытым окнам команду закрыться, чтобы после выхода
	// не остались висеть дочерние процессы.
	settings.StopWindows()
	exit()

	windows.wait(windowCloseTimeout)
}

// windowTracker не даёт открыть одно и то же окно несколько раз.
type windowTracker struct {
	mu     sync.Mutex
	open   map[windowKind]bool
	closed sync.WaitGroup
}

func newWindowTracker() *windowTracker {
	return &windowTracker{
		open: make(map[windowKind]bool),
	}
}

// tryOpen помечает окно открытым. Возвращает false, если оно уже открыто.
func (w *windowTracker) tryOpen(kind windowKind) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.open[kind] {
		return false
	}

	w.open[kind] = true
	w.closed.Add(1)

	return true
}

// close снимает отметку об окне.
func (w *windowTracker) close(kind windowKind) {
	w.mu.Lock()
	if w.open[kind] {
		delete(w.open, kind)
	}
	w.mu.Unlock()

	w.closed.Done()
}

// wait ждёт закрытия всех дочерних окон, но не дольше указанного времени.
// При выходе это нужно, чтобы дочерние окна (Fyne) успели завершиться
// вместе с программой, а не остались висеть.
func (w *windowTracker) wait(timeout time.Duration) {
	done := make(chan struct{})

	go func() {
		w.closed.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// openURL открывает адрес в браузере по умолчанию.
func openURL(address string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Start()
	case "darwin":
		return exec.Command("open", address).Start()
	default:
		return exec.Command("xdg-open", address).Start()
	}
}

func openFolder(path string) error {
	if path == "" {
		return fmt.Errorf("папка хранилища не задана")
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// Address собирает адрес вида http://ip:port.
func Address(ip string, port int) string {
	return fmt.Sprintf("http://%s:%d", ip, port)
}
