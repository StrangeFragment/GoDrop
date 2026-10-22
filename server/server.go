package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"GoDrop/config"
	"GoDrop/storage"
)

const (
	// preferredPorts — порты, которые пробуются, если сохранённый порт занят.
	preferredPorts = "666, 777, 888, 999"

	// MaxUploadBytes — максимальный размер одного запроса на загрузку (8 ГиБ).
	MaxUploadBytes int64 = 8 << 30

	// multipartMemory — сколько данных держать в памяти, остальное уходит
	// во временные файлы.
	multipartMemory int64 = 32 << 20

	readHeaderTimeout = 15 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 1 << 20
)

var ports = []int{666, 777, 888, 999}

// Server — запущенный HTTP-сервер.
type Server struct {
	HTTPServer *http.Server
	Listener   net.Listener
	Port       int
}

// URL возвращает адрес сервера для указанного IP.
func (s *Server) URL(ip string) string {
	return fmt.Sprintf("http://%s:%d", ip, s.Port)
}

// Start поднимает HTTP-сервер. Порт берётся из настроек; если он занят,
// подбирается свободный. Ошибка возвращается честно: если порт не удалось
// открыть, сервер не считается запущенным.
func Start(cfg config.Config) (*Server, error) {
	if cfg.StorageDir == "" {
		return nil, errors.New("папка хранилища не задана")
	}

	store := storage.New(cfg.StorageDir)

	// Проверяем папку заранее, чтобы не узнать о проблеме при первой загрузке.
	if _, err := store.TodayDir(); err != nil {
		return nil, err
	}

	handler := &Handler{
		Storage:           store,
		AllowedExtensions: cfg.AllowedExtensions,
		MaxUploadBytes:    MaxUploadBytes,
		MultipartMemory:   multipartMemory,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler.Index)
	mux.HandleFunc("/upload", handler.Upload)
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/favicon.ico", handler.Favicon)
	mux.HandleFunc("/icon-192.png", handler.Icon192)
	mux.HandleFunc("/icon-512.png", handler.Icon512)
	mux.HandleFunc("/apple-touch-icon.png", handler.AppleTouchIcon)
	mux.HandleFunc("/site.webmanifest", handler.Manifest)

	listener, port, err := listen(cfg.Port)
	if err != nil {
		return nil, err
	}

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          log.Default(),
	}

	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil &&
			!errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("HTTP-сервер остановлен с ошибкой: %v", serveErr)
		}
	}()

	return &Server{
		HTTPServer: httpServer,
		Listener:   listener,
		Port:       port,
	}, nil
}

// listen открывает слушающий сокет: сначала на сохранённом порту,
// затем на списке популярных портов, затем на любом свободном порту,
// который выделит система.
func listen(saved int) (net.Listener, int, error) {
	candidates := make([]int, 0, len(ports)+1)

	if saved >= 1 && saved <= 65535 {
		candidates = append(candidates, saved)
	}

	for _, port := range ports {
		if port != saved {
			candidates = append(candidates, port)
		}
	}

	var lastErr error

	for _, port := range candidates {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return listener, port, nil
		}

		lastErr = err
	}

	// Последняя попытка: порт выбирает сама операционная система.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, 0, fmt.Errorf(
			"не удалось открыть порт (проверены %d, %s и произвольный): %w",
			saved, preferredPorts, errors.Join(lastErr, err),
		)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, errors.New("не удалось определить номер свободного порта")
	}

	return listener, addr.Port, nil
}
