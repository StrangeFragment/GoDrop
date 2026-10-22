package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"GoDrop/autostart"
	"GoDrop/config"
	"GoDrop/server"
	"GoDrop/settings"
	"GoDrop/singleinstance"
	"GoDrop/tray"
)

const (
	shutdownTimeout = 30 * time.Second

	// appLockName — имя блокировки, не дающей запустить вторую копию GoDrop.
	appLockName = "GoDrop.App"

	// qrStateFile — файл, через который окно с QR-кодом получает адрес.
	qrStateFile = "godrop-qr.json"
)

// appState — текущее состояние запущенной программы.
// Сервер и настройки меняются при сохранении настроек, поэтому доступ
// к ним защищён мьютексом.
type appState struct {
	mu      sync.Mutex
	running *server.Server
	config  config.Config
}

func (s *appState) server() *server.Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

func (s *appState) update(running *server.Server, cfg config.Config) {
	s.mu.Lock()
	s.running = running
	s.config = cfg
	s.mu.Unlock()
}

func (s *appState) currentConfig() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.config
}

func setupLogging() (*os.File, error) {
	logPath, err := config.LogPath()
	if err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(
		logPath,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"не удалось открыть файл лога %q: %w",
			logPath,
			err,
		)
	}

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.Ldate | log.Ltime)

	log.Printf("запуск GoDrop")
	log.Printf("файл лога: %s", logPath)

	// Чистим старые логи, чтобы папка не разрасталась.
	config.PruneOldLogs(config.DefaultLogRetentionDays)

	return logFile, nil
}

func main() {
	// Дочерние режимы (окна) не должны брать блокировку приложения
	// и не должны писать в общий лог: они запускаются из уже
	// работающего экземпляра.
	if runChildMode() {
		return
	}

	logFile, err := setupLogging()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		log.Printf("завершение GoDrop")

		if err := logFile.Close(); err != nil {
			fmt.Printf("не удалось закрыть файл лога: %v\n", err)
		}
	}()

	lock, err := acquireAppLock()
	if err != nil {
		log.Println(err)
		fmt.Println(err)
		return
	}
	defer lock.Release()

	// Окна (дочерние процессы) проверяют этот файл и закрываются
	// вместе с программой; остаток от прошлого запуска удаляем.
	settings.ClearStopSignal()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("настройки загружены со значениями по умолчанию: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Printf("некорректные настройки (%v), используются значения по умолчанию", err)

		cfg = config.Default()
	}

	// Если автозапуск включён в настройках, обновляем путь в системе:
	// программа могла переехать в другую папку.
	if cfg.StartWithWindows {
		if err := autostart.Set(true); err != nil {
			log.Printf("не удалось включить автозапуск: %v", err)
		}
	}

	log.Printf("папка хранения: %s", cfg.StorageDir)
	log.Printf("настройки загружены, порт: %d", cfg.Port)

	running, err := server.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("HTTP-сервер запущен на порту: %d", running.Port)

	url := running.URL(localIPv4())

	fmt.Println("GoDrop запущен")
	fmt.Println("Адрес:", url)

	state := &appState{}
	state.update(running, cfg)

	tray.Run(tray.Callbacks{
		URL:        url,
		StorageDir: cfg.StorageDir,
		Config:     cfg,

		OnSettingsSaved: func(nextConfig config.Config) (string, error) {
			fmt.Println("Применение новых настроек...")
			log.Printf("применение новых настроек")

			return applySettings(state, nextConfig)
		},

		OnExit: func() {
			log.Printf("получена команда выхода")

			shutdownServer(state.server())

			// Просим открытые окна закрыться, иначе они останутся
			// висеть после выхода из программы.
			settings.StopWindows()

			qrFile := filepath.Join(os.TempDir(), qrStateFile)
			if err := os.Remove(qrFile); err != nil && !os.IsNotExist(err) {
				log.Printf("не удалось удалить временный QR-файл: %v", err)
			}
		},
	})
}

// runChildMode обрабатывает запуск в режиме окна.
// Возвращает true, если программа работала как дочерний процесс.
func runChildMode() bool {
	if len(os.Args) < 3 {
		return false
	}

	var err error

	switch os.Args[1] {
	case "--settings":
		err = settings.RunFile(os.Args[2])
	case "--qr":
		err = settings.RunQRFile(os.Args[2])
	default:
		return false
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return true
}

// acquireAppLock не даёт запустить две копии: иначе на компьютере
// появились бы два значка в трее и две конкурирующие папки хранилища.
func acquireAppLock() (*singleinstance.Lock, error) {
	lock, ok, err := singleinstance.Acquire(appLockName)
	if err != nil {
		return nil, fmt.Errorf("не удалось проверить запущенные копии: %w", err)
	}

	if !ok {
		return nil, errors.New(
			"GoDrop уже запущен — вторую копию открывать не нужно",
		)
	}

	return lock, nil
}

// applySettings останавливает сервер, применяет новые настройки и запускает
// его заново. Если новый сервер поднять не удалось — возвращает прежние
// настройки в работу, чтобы программа не осталась без сервера.
func applySettings(state *appState, nextConfig config.Config) (string, error) {
	previous := state.currentConfig()
	current := state.server()

	shutdownServer(current)

	nextServer, err := server.Start(nextConfig)
	if err != nil {
		log.Printf("не удалось запустить сервер с новыми настройками: %v", err)

		restored, restoreErr := server.Start(previous)
		if restoreErr != nil {
			state.update(nil, previous)

			log.Printf("не удалось вернуть прежние настройки: %v", restoreErr)

			return "", fmt.Errorf(
				"не удалось применить настройки: %v",
				err,
			)
		}

		state.update(restored, previous)

		return "", fmt.Errorf(
			"не удалось применить настройки (%v). Оставлены прежние: порт %d",
			err,
			restored.Port,
		)
	}

	cfg := nextConfig
	cfg.Normalize()

	if err := config.Save(cfg); err != nil {
		log.Printf("не удалось сохранить config.json: %v", err)
	}

	if err := autostart.Set(cfg.StartWithWindows); err != nil {
		log.Printf("не удалось изменить автозапуск: %v", err)
	}

	nextURL := nextServer.URL(localIPv4())

	state.update(nextServer, cfg)

	fmt.Println("Настройки применены")
	fmt.Println("Адрес:", nextURL)

	log.Printf(
		"настройки применены, порт: %d, папка: %s",
		nextServer.Port,
		cfg.StorageDir,
	)

	return nextURL, nil
}

// shutdownServer корректно останавливает HTTP-сервер, при неудаче — принудительно закрывает.
func shutdownServer(current *server.Server) {
	if current == nil || current.HTTPServer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := current.HTTPServer.Shutdown(ctx); err != nil {
		log.Printf("ошибка корректной остановки HTTP-сервера: %v", err)

		if closeErr := current.HTTPServer.Close(); closeErr != nil {
			log.Printf("ошибка принудительной остановки HTTP-сервера: %v", closeErr)
		}

		return
	}

	log.Printf("HTTP-сервер остановлен")
}

// localIPv4 ищет адрес компьютера в локальной сети.
func localIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("не удалось получить сетевые интерфейсы: %v", err)
		return "127.0.0.1"
	}

	var fallback string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, address := range addresses {
			var ip net.IP

			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}

			if ip == nil {
				continue
			}

			ipv4 := ip.To4()
			if ipv4 == nil || ipv4.IsLoopback() {
				continue
			}

			// Приватные адреса — самая вероятная "своя" сеть.
			if ipv4.IsPrivate() {
				return ipv4.String()
			}

			if fallback == "" {
				fallback = ipv4.String()
			}
		}
	}

	if fallback != "" {
		return fallback
	}

	return "127.0.0.1"
}
