package web

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

type OverseerServer struct {
	httpServer *http.Server
}

func NewServer(mux *http.ServeMux) *OverseerServer {
	return &OverseerServer{
		httpServer: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1 MiB
		},
	}
}

// StartOnPort запускает сервер на конкретном, заранее проверенном мутексом порту
func (s *OverseerServer) StartOnPort(port int) (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("не удалось запустить сервер на адресе %s: %v", addr, err)
	}

	s.httpServer.Addr = addr

	go func() {
		log.Printf("Веб-сервер GoOverseer успешно запущен на http://%s", s.httpServer.Addr)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Критическая ошибка HTTP-сервера: %v", err)
		}
	}()

	return fmt.Sprintf("http://127.0.0.1:%d", port), nil
}

func (s *OverseerServer) Shutdown() error {
	log.Println("Завершение работы веб-сервера...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}
