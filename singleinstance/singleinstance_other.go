//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package singleinstance

// acquire — заглушка для платформ без поддержки блокировок.
// Программа работает, но защита от повторного запуска недоступна.
func acquire(name string) (*Lock, bool, error) {
	_ = name

	return &Lock{release: func() {}}, true, nil
}
