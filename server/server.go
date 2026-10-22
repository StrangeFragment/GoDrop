package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"GoDrop/config"
	"GoDrop/storage"
)

var ports = []int{666, 777, 888, 999}

type Server struct {
	HTTPServer *http.Server
	Port       int
}

func Start(cfg config.Config) (*Server, error) {
	port := findPort(cfg.Port)

	if port == 0 {
		port = 8080
		if !isPortAvailable(port) {
			return nil, fmt.Errorf("не удалось найти свободный порт")
		}
	}

	store := storage.New(cfg.StorageDir)

	handler := &Handler{
		Storage:           store,
		AllowedExtensions: cfg.AllowedExtensions,
	}

	mux := http.NewServeMux()

	/* 	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./favicon.ico")
	}) */

	mux.HandleFunc("/", handler.Index)
	mux.HandleFunc("/upload", handler.Upload)
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/favicon.ico", handler.Favicon)

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}

	go func() {
		_ = httpServer.ListenAndServe()
	}()

	return &Server{
		HTTPServer: httpServer,
		Port:       port,
	}, nil
}

func findPort(saved int) int {
	if saved != 0 && isPortAvailable(saved) {
		return saved
	}

	for _, port := range ports {
		if isPortAvailable(port) {
			return port
		}
	}

	return 0
}

func isPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	_ = listener.Close()
	return true
}
