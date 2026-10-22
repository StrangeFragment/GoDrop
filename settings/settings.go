package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

	"GoDrop/config"
)

type settingsRequest struct {
	Config config.Config `json:"config"`
}

type settingsResponse struct {
	Config config.Config `json:"config"`
	OK     bool          `json:"ok"`
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

var qrStateFile = filepath.Join(os.TempDir(), "godrop-qr.json")

func Show(current config.Config) (config.Config, bool) {
	filename, err := os.CreateTemp("", "godrop-settings-*.json")
	if err != nil {
		return current, false
	}

	filenamePath := filename.Name()
	_ = filename.Close()
	defer os.Remove(filenamePath)

	if err := writeJSON(filenamePath, settingsRequest{Config: current}); err != nil {
		return current, false
	}

	executable, err := os.Executable()
	if err != nil {
		return current, false
	}

	command := exec.Command(executable, "--settings", filenamePath)
	if err := command.Run(); err != nil {
		return current, false
	}

	data, err := os.ReadFile(filenamePath)
	if err != nil {
		return current, false
	}

	var response settingsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return current, false
	}

	return response.Config, response.OK
}

func ShowQR(url string) error {
	if err := writeJSON(qrStateFile, qrRequest{URL: url}); err != nil {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}

	return exec.Command(executable, "--qr", qrStateFile).Start()
}

func UpdateQR(url string) error {
	return writeJSON(qrStateFile, qrRequest{URL: url})
}

func RunFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var request settingsRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}

	return writeJSON(filename, showWindow(request.Config))
}

func RunQRFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var request qrRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}

	showQRWindow(request.URL)
	return nil
}

// newBorderlessWindow создаёт окно без системного заголовка и рамки.
func newBorderlessWindow(a fyne.App) fyne.Window {
	driver := a.Driver().(desktop.Driver)
	return driver.CreateSplashWindow()
}

func showQRWindow(url string) {
	a := app.NewWithID("GoDrop.QR")
	a.Settings().SetTheme(goDropTheme{})

	window := newBorderlessWindow(a)
	window.Resize(fyne.NewSize(520, 620))

	title := canvas.NewText(
		"GoDrop!",
		color.NRGBA{R: 4, G: 170, B: 109, A: 255},
	)
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := widget.NewLabel("Отсканируйте QR-код и перейдите по ссылке для передачи файлов")
	subtitle.Alignment = fyne.TextAlignCenter

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
			return
		}

		data, err = roundQRImage(data, 16)
		if err != nil {
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

	window.SetContent(container.NewPadded(container.NewVBox(
		title,
		subtitle,
		container.NewCenter(qrImage),
		addressLabel,
		container.NewCenter(closeButtonWithFixedWidth),
	)))

	window.SetCloseIntercept(closeWindow)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		lastURL := url

		for {
			select {
			case <-ticker.C:
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

	window.CenterOnScreen()
	window.Show()
	a.Run()
}

func showWindow(current config.Config) settingsResponse {
	a := app.NewWithID("GoDrop.Settings")
	a.Settings().SetTheme(goDropTheme{})

	window := newBorderlessWindow(a)
	window.Resize(fyne.NewSize(560, 430))

	title := canvas.NewText(
		"Настройки GoDrop",
		color.NRGBA{R: 4, G: 170, B: 109, A: 255},
	)
	title.TextSize = 24
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	storageEntry := widget.NewEntry()
	storageEntry.SetText(current.StorageDir)

	portEntry := widget.NewEntry()
	portEntry.SetText(strconv.Itoa(current.Port))

	extensionsEntry := widget.NewEntry()
	extensionsEntry.SetText(strings.Join(current.AllowedExtensions, ", "))

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

	selectFolderButton := widget.NewButton("Выбрать папку", func() {
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
		container.NewHBox(saveButton, cancelButton),
	)

	window.SetContent(container.NewPadded(form))
	window.SetCloseIntercept(func() {
		finish(current, false)
	})

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

	return os.WriteFile(filename, data, 0600)
}

func readConfig(
	previous config.Config,
	storageText string,
	portText string,
	extensionsText string,
	startWithWindows bool,
) (config.Config, error) {
	storageDir := strings.TrimSpace(storageText)
	if storageDir == "" {
		return previous, fmt.Errorf("папка хранилища не может быть пустой")
	}

	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port < 1 || port > 65535 {
		return previous, fmt.Errorf("порт должен быть числом от 1 до 65535")
	}

	extensions := parseExtensions(extensionsText)
	if len(extensions) == 0 {
		return previous, fmt.Errorf("укажите хотя бы одно разрешённое расширение")
	}

	return config.Config{
		StorageDir:        filepath.Clean(storageDir),
		Port:              port,
		StartWithWindows:  startWithWindows,
		AllowedExtensions: extensions,
	}, nil
}

func parseExtensions(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' ||
			r == ';' ||
			r == ' ' ||
			r == '\n' ||
			r == '\t'
	})

	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)

	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}

		if !strings.HasPrefix(part, ".") {
			part = "." + part
		}

		if !seen[part] {
			seen[part] = true
			result = append(result, part)
		}
	}

	return result
}
