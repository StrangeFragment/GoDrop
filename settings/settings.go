package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/skip2/go-qrcode"

	"GoDrop/assets"
	"GoDrop/config"
	"GoDrop/singleinstance"
)

const (
	// settingsLockName — блокировка окна настроек.
	settingsLockName = "GoDrop.SettingsWindow"

	// qrLockName — блокировка окна с QR-кодом.
	qrLockName = "GoDrop.QRWindow"

	// pollInterval — как часто окна проверяют обновления и команду закрытия.
	pollInterval = 500 * time.Millisecond
)

type settingsRequest struct {
	Config config.Config `json:"config"`
}

type settingsResponse struct {
	Config config.Config `json:"config"`
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
}

type qrRequest struct {
	URL string `json:"url"`
}

type goDropTheme struct{}

func (goDropTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 15, G: 15, B: 26, A: 255} // #0f0f1a
	case theme.ColorNameForeground:
		return color.NRGBA{R: 232, G: 232, B: 240, A: 255} // #e8e8f0
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 23, G: 105, B: 255, A: 255} // #1769ff
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 25, G: 25, B: 39, A: 255} // #191927
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 136, G: 136, B: 170, A: 255} // #8888aa
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 55, G: 55, B: 75, A: 255}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (goDropTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (goDropTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (goDropTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

var (
	qrStateFile = filepath.Join(os.TempDir(), "godrop-qr.json")
	stopFile    = filepath.Join(os.TempDir(), "godrop-stop")
)

// ClearStopSignal убирает файл-команду "закрыть все окна".
// Вызывается при старте программы.
func ClearStopSignal() {
	_ = os.Remove(stopFile)
}

// StopWindows просит все открытые окна закрыться.
// Окна — отдельные процессы, поэтому общаемся с ними через файл.
func StopWindows() {
	_ = os.WriteFile(stopFile, []byte("stop"), 0o600)
}

func stopRequested() bool {
	_, err := os.Stat(stopFile)

	return err == nil
}

// Show открывает окно настроек и возвращает новые настройки.
//
// Функция блокируется до закрытия окна, поэтому вызывать её нужно
// из отдельной горутины.
func Show(current config.Config) (config.Config, bool, error) {
	filename, err := os.CreateTemp("", "godrop-settings-*.json")
	if err != nil {
		return current, false, fmt.Errorf("не удалось создать временный файл: %w", err)
	}

	filenamePath := filename.Name()
	_ = filename.Close()

	defer func() {
		if err := os.Remove(filenamePath); err != nil && !os.IsNotExist(err) {
			log.Printf("не удалось удалить временный файл настроек: %v", err)
		}
	}()

	if err := writeJSON(filenamePath, settingsRequest{Config: current}); err != nil {
		return current, false, err
	}

	executable, err := os.Executable()
	if err != nil {
		return current, false, fmt.Errorf("не удалось определить путь к программе: %w", err)
	}

	command := exec.Command(executable, "--settings", filenamePath)
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return current, false, fmt.Errorf(
				"окно настроек закрылось с ошибкой (код %d)",
				exitErr.ExitCode(),
			)
		}

		return current, false, fmt.Errorf("не удалось открыть окно настроек: %w", err)
	}

	data, err := os.ReadFile(filenamePath)
	if err != nil {
		return current, false, fmt.Errorf("не удалось прочитать ответ окна настроек: %w", err)
	}

	var response settingsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return current, false, fmt.Errorf("не удалось разобрать ответ окна настроек: %w", err)
	}

	if !response.OK {
		if response.Error != "" {
			return current, false, errors.New(response.Error)
		}

		return current, false, nil
	}

	response.Config.Normalize()

	if err := response.Config.Validate(); err != nil {
		return current, false, err
	}

	return response.Config, true, nil
}

// ShowQR открывает окно с QR-кодом. Блокируется до закрытия окна.
func ShowQR(url string) error {
	if err := writeJSON(qrStateFile, qrRequest{URL: url}); err != nil {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}

	command := exec.Command(executable, "--qr", qrStateFile)
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf(
				"окно с QR-кодом закрылось с ошибкой (код %d)",
				exitErr.ExitCode(),
			)
		}

		return err
	}

	return nil
}

// UpdateQR сообщает открытому окну с QR-кодом новый адрес.
// Если окно закрыто, вызов ничего не ломает.
func UpdateQR(url string) error {
	return writeJSON(qrStateFile, qrRequest{URL: url})
}

// RunFile — режим "--settings": показывает окно настроек и записывает
// результат обратно в файл, созданный основным процессом.
func RunFile(filename string) error {
	lock, ok, err := singleinstance.Acquire(settingsLockName)
	if err != nil {
		return err
	}

	if !ok {
		// Окно уже открыто: второй раз не показываем.
		return nil
	}
	defer lock.Release()

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var request settingsRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}

	request.Config.Normalize()

	return writeJSON(filename, showWindow(request.Config))
}

// RunQRFile — режим "--qr": показывает окно с QR-кодом.
func RunQRFile(filename string) error {
	lock, ok, err := singleinstance.Acquire(qrLockName)
	if err != nil {
		return err
	}

	if !ok {
		return nil
	}
	defer lock.Release()

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var request qrRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}

	if request.URL == "" {
		return fmt.Errorf("адрес для QR-кода пуст")
	}

	showQRWindow(request.URL)

	return nil
}

// newBorderlessWindow создаёт окно без системного заголовка и рамки.
func newBorderlessWindow(a fyne.App) fyne.Window {
	driver, ok := a.Driver().(desktop.Driver)
	if !ok {
		return a.NewWindow("GoDrop")
	}

	return driver.CreateSplashWindow()
}

// appIcon возвращает значок приложения для окна и панели задач.
func appIcon() fyne.Resource {
	return fyne.NewStaticResource("godrop.png", assets.GoDrop256)
}

// fitWindow подгоняет размер окна под содержимое и делает его изменяемым
// только в разумных пределах.
func fitWindow(window fyne.Window, content fyne.CanvasObject, minSize fyne.Size) {
	size := content.MinSize()

	if size.Width < minSize.Width {
		size.Width = minSize.Width
	}

	if size.Height < minSize.Height {
		size.Height = minSize.Height
	}

	window.Resize(size)
}

func showQRWindow(url string) {
	a := app.NewWithID("GoDrop.QR")
	a.Settings().SetTheme(goDropTheme{})
	a.SetIcon(appIcon())

	window := newBorderlessWindow(a)
	window.SetTitle("GoDrop! — QR-код")
	window.SetIcon(appIcon())

	title := canvas.NewText(
		"GoDrop!",
		color.NRGBA{R: 4, G: 170, B: 109, A: 255},
	)
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := widget.NewLabel(
		"Отсканируйте QR-код и перейдите по ссылке для передачи файлов",
	)
	subtitle.Alignment = fyne.TextAlignCenter
	subtitle.Wrapping = fyne.TextWrapWord

	qrImage := canvas.NewImageFromResource(nil)
	qrImage.FillMode = canvas.ImageFillContain
	qrImage.SetMinSize(fyne.NewSize(420, 420))

	addressLabel := canvas.NewText(
		"",
		color.NRGBA{R: 4, G: 170, B: 109, A: 255},
	)
	addressLabel.TextSize = 18
	addressLabel.Alignment = fyne.TextAlignCenter
	addressLabel.SetMinSize(fyne.NewSize(420, 28))

	update := func(nextURL string) {
		data, err := qrcode.Encode(nextURL, qrcode.Medium, 420)
		if err != nil {
			log.Printf("не удалось построить QR-код: %v", err)
			return
		}

		data, err = roundQRImage(data, 16)
		if err != nil {
			log.Printf("не удалось скруглить QR-код: %v", err)
			return
		}

		qrImage.Resource = fyne.NewStaticResource("godrop-qr.png", data)
		qrImage.Refresh()

		addressLabel.Text = nextURL
		addressLabel.Refresh()
	}

	update(url)

	done := make(chan struct{})
	var closeOnce sync.Once

	closeWindow := func() {
		closeOnce.Do(func() {
			close(done)
			a.Quit()
		})
	}

	closeButton := widget.NewButton("Закрыть", closeWindow)
	closeButton.Importance = widget.HighImportance

	closeButtonWithFixedWidth := container.NewGridWrap(
		fyne.NewSize(420, 48),
		closeButton,
	)

	hint := widget.NewLabel(
		"Телефон и компьютер должны быть в одной Wi-Fi сети.\n" +
			"Если страница не открывается — разрешите GoDrop доступ\n" +
			"в брандмауэре Windows.",
	)
	hint.Alignment = fyne.TextAlignCenter
	hint.Wrapping = fyne.TextWrapWord
	hint.TextStyle = fyne.TextStyle{Italic: true}

	content := container.NewPadded(container.NewVBox(
		title,
		subtitle,
		container.NewCenter(qrImage),
		container.NewCenter(addressLabel),
		container.NewCenter(closeButtonWithFixedWidth),
		hint,
	))

	window.SetContent(content)
	window.SetCloseIntercept(closeWindow)
	window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			closeWindow()
		}
	})

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		lastURL := url

		for {
			select {
			case <-ticker.C:
				// Просьба закрыться (например, выход из программы).
				if stopRequested() {
					fyne.Do(closeWindow)
					return
				}

				data, err := os.ReadFile(qrStateFile)
				if err != nil {
					continue
				}

				var request qrRequest
				if err := json.Unmarshal(data, &request); err != nil {
					continue
				}

				if request.URL == "" || request.URL == lastURL {
					continue
				}

				lastURL = request.URL

				fyne.Do(func() {
					update(request.URL)
				})

			case <-done:
				return
			}
		}
	}()

	fitWindow(window, content, fyne.NewSize(520, 620))
	window.CenterOnScreen()
	window.Show()
	a.Run()
}

func showWindow(current config.Config) settingsResponse {
	a := app.NewWithID("GoDrop.Settings")
	a.Settings().SetTheme(goDropTheme{})
	a.SetIcon(appIcon())

	window := newBorderlessWindow(a)
	window.SetTitle("GoDrop! — настройки")
	window.SetIcon(appIcon())

	title := canvas.NewText(
		"Настройки GoDrop",
		color.NRGBA{R: 4, G: 170, B: 109, A: 255},
	)
	title.TextSize = 24
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	storageEntry := widget.NewEntry()
	storageEntry.SetText(current.StorageDir)
	storageEntry.SetPlaceHolder(`Например: C:\Users\Имя\Downloads\GoDrop`)

	portEntry := widget.NewEntry()
	portEntry.SetText(fmt.Sprintf("%d", current.Port))

	extensionsEntry := widget.NewEntry()
	extensionsEntry.SetText(current.ExtensionsString())
	extensionsEntry.SetPlaceHolder(".jpg, .png, .pdf")

	startCheck := widget.NewCheck("Запускать вместе с Windows", nil)
	startCheck.SetChecked(current.StartWithWindows)

	var (
		resultOnce sync.Once
		response   = settingsResponse{
			Config: current,
			OK:     false,
		}
	)

	finish := func(nextConfig config.Config, ok bool) {
		resultOnce.Do(func() {
			response.Config = nextConfig
			response.OK = ok
			a.Quit()
		})
	}

	selectFolderButton := widget.NewButton("Выбрать", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}

			if uri != nil {
				storageEntry.SetText(uri.Path())
			}
		}, window)
	})

	saveButton := widget.NewButton("Сохранить", func() {
		nextConfig, err := readConfig(
			current,
			storageEntry.Text,
			portEntry.Text,
			extensionsEntry.Text,
			startCheck.Checked,
		)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}

		// Папку создаём сразу, чтобы ошибка прав доступа
		// всплыла здесь, а не при первой загрузке с телефона.
		if err := os.MkdirAll(nextConfig.StorageDir, 0o755); err != nil {
			dialog.ShowError(
				fmt.Errorf("не удалось создать папку хранилища: %w", err),
				window,
			)
			return
		}

		finish(nextConfig, true)
	})
	saveButton.Importance = widget.HighImportance

	cancelButton := widget.NewButton("Отмена", func() {
		finish(current, false)
	})

	form := container.NewVBox(
		title,
		widget.NewLabel("Папка хранилища"),
		container.NewBorder(nil, nil, nil, selectFolderButton, storageEntry),
		widget.NewLabel("Порт"),
		portEntry,
		widget.NewLabel("Разрешённые расширения"),
		extensionsEntry,
		startCheck,
		container.NewGridWithColumns(2, cancelButton, saveButton),
	)

	content := container.NewPadded(form)

	window.SetContent(content)
	window.SetCloseIntercept(func() {
		finish(current, false)
	})
	window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyEscape:
			finish(current, false)
		case fyne.KeyReturn, fyne.KeyEnter:
			saveButton.OnTapped()
		}
	})

	// Следим за командой закрытия от основного процесса.
	stopWatcher := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if stopRequested() {
					fyne.Do(func() { finish(current, false) })
					return
				}
			case <-stopWatcher:
				return
			}
		}
	}()
	defer close(stopWatcher)

	fitWindow(window, content, fyne.NewSize(560, 430))
	window.CenterOnScreen()
	window.Show()
	a.Run()

	return response
}

// roundQRImage скругляет внешние углы QR-изображения,
// делая пиксели за пределами скругления прозрачными.
func roundQRImage(data []byte, radius int) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	width := bounds.Dx()
	height := bounds.Dy()

	if radius > 0 {
		r := float64(radius)
		radiusSquared := r * r

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				cornerX := x
				if x >= width-radius {
					cornerX = width - 1 - x
				}

				cornerY := y
				if y >= height-radius {
					cornerY = height - 1 - y
				}

				if cornerX < radius && cornerY < radius {
					dx := r - float64(cornerX)
					dy := r - float64(cornerY)

					if dx*dx+dy*dy > radiusSquared {
						dst.SetNRGBA(
							bounds.Min.X+x,
							bounds.Min.Y+y,
							color.NRGBA{},
						)
					}
				}
			}
		}
	}

	var result bytes.Buffer
	if err := png.Encode(&result, dst); err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

func writeJSON(filename string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0o600)
}

func readConfig(
	previous config.Config,
	storageText string,
	portText string,
	extensionsText string,
	startWithWindows bool,
) (config.Config, error) {
	next := previous

	next.StorageDir = filepath.Clean(storageText)
	if storageText == "" || next.StorageDir == "." {
		return previous, fmt.Errorf("папка хранилища не может быть пустой")
	}

	port, err := config.ParsePort(portText)
	if err != nil {
		return previous, err
	}
	next.Port = port

	extensions := config.ParseExtensions(extensionsText)
	if len(extensions) == 0 {
		return previous, fmt.Errorf("укажите хотя бы одно разрешённое расширение")
	}
	next.AllowedExtensions = extensions

	next.StartWithWindows = startWithWindows

	if err := next.Validate(); err != nil {
		return previous, err
	}

	return next, nil
}
