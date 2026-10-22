package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"GoDrop/config"
	"GoDrop/server"
	"GoDrop/settings"
	"GoDrop/tray"
)

const shutdownTimeout = 30 * time.Second

func setupLogging() (*os.File, error) {
	logPath, err := config.LogPath()
	if err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(
		logPath,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
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

	return logFile, nil
}

func main() {
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

	if len(os.Args) >= 3 && os.Args[1] == "--settings" {
		if err := settings.RunFile(os.Args[2]); err != nil {
			log.Println(err)
			os.Exit(1)
		}

		return
	}

	if len(os.Args) >= 3 && os.Args[1] == "--qr" {
		if err := settings.RunQRFile(os.Args[2]); err != nil {
			log.Println(err)
			os.Exit(1)
		}

		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("папка хранения: %s", cfg.StorageDir)
	log.Printf("настройки загружены, порт: %d", cfg.Port)

	running, err := server.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("HTTP-сервер запущен на порту: %d", running.Port)

	if cfg.Port != running.Port {
		cfg.Port = running.Port

		if err := config.Save(cfg); err != nil {
			log.Printf("не удалось сохранить порт: %v", err)
		}
	}

	ip := localIPv4()
	url := tray.Address(ip, running.Port)

	fmt.Println("GoDrop запущен")
	fmt.Println("Адрес:", url)

	var serverMu sync.Mutex
	var stopped bool

	stopHTTPServer := func(current *server.Server) {
		if current == nil || current.HTTPServer == nil {
			return
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			shutdownTimeout,
		)
		defer cancel()

		if err := current.HTTPServer.Shutdown(ctx); err != nil {
			log.Printf("ошибка корректной остановки HTTP-сервера: %v", err)

			if closeErr := current.HTTPServer.Close(); closeErr != nil {
				log.Printf(
					"ошибка принудительной остановки HTTP-сервера: %v",
					closeErr,
				)
			}

			return
		}

		log.Printf("HTTP-сервер остановлен")
	}

	stopServer := func() {
		serverMu.Lock()

		if stopped || running == nil {
			serverMu.Unlock()
			return
		}

		stopped = true
		current := running

		serverMu.Unlock()

		stopHTTPServer(current)
	}

	tray.Run(tray.Callbacks{
		URL:        url,
		StorageDir: cfg.StorageDir,
		Config:     cfg,

		OnSettingsSaved: func(nextConfig config.Config) (string, error) {
			fmt.Println("Применение новых настроек...")
			log.Printf("применение новых настроек")

			serverMu.Lock()
			currentServer := running
			serverMu.Unlock()

			stopHTTPServer(currentServer)

			nextServer, err := server.Start(nextConfig)
			if err != nil {
				log.Printf("не удалось перезапустить сервер: %v", err)

				return "", fmt.Errorf(
					"не удалось перезапустить сервер: %w",
					err,
				)
			}

			serverMu.Lock()
			running = nextServer
			stopped = false
			serverMu.Unlock()

			cfg = nextConfig
			cfg.Port = running.Port

			if err := config.Save(cfg); err != nil {
				log.Printf("не удалось сохранить config.json: %v", err)
			}

			nextURL := tray.Address(localIPv4(), running.Port)

			fmt.Println("Настройки применены")
			fmt.Println("Адрес:", nextURL)

			log.Printf(
				"настройки применены, порт: %d, папка: %s",
				running.Port,
				cfg.StorageDir,
			)

			return nextURL, nil
		},

		OnExit: func() {
			log.Printf("получена команда выхода")

			stopServer()

			qrFile := filepath.Join(os.TempDir(), "godrop-qr.json")

			if err := os.Remove(qrFile); err != nil && !os.IsNotExist(err) {
				log.Printf(
					"не удалось удалить временный QR-файл: %v",
					err,
				)
			}
		},
	})
}

func localIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("не удалось получить сетевые интерфейсы: %v", err)
		return "127.0.0.1"
	}

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

			if ipv4 := ip.To4(); ipv4 != nil {
				return ipv4.String()
			}
		}
	}

	return "127.0.0.1"
}

func init() {
	_ = os.Setenv("FYNE_FONT", "")
}
