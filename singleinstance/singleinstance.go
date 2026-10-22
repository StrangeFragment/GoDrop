// Package singleinstance не даёт открыть одно и то же окно (или само
// приложение) несколько раз.
//
// На Windows используется именованный мьютекс (CreateMutexW), на Unix —
// flock на файле блокировки. Оба варианта освобождаются автоматически,
// даже если процесс завершился аварийно.
package singleinstance

import (
	"errors"
	"fmt"
)

// Lock — удерживаемая блокировка. Пока объект жив, второй экземпляр
// с тем же именем получить блокировку не сможет.
type Lock struct {
	release func()
}

// Release освобождает блокировку. Повторные вызовы безопасны.
func (l *Lock) Release() {
	if l == nil || l.release == nil {
		return
	}

	release := l.release
	l.release = nil
	release()
}

// Acquire пытается получить эксклюзивную блокировку с указанным именем.
//
// Возвращает:
//   - lock, true  — блокировка получена;
//   - nil, false  — имя уже занято другим процессом;
//   - nil, false + error — внутренняя ошибка.
func Acquire(name string) (*Lock, bool, error) {
	if name == "" {
		return nil, false, fmt.Errorf("пустое имя блокировки")
	}

	return acquire(name)
}

// AcquireOrError — упрощённый вариант для случаев, когда "уже запущено"
// не считается ошибкой, но нужно понимать, что делать дальше.
func AcquireOrError(name string) (*Lock, error) {
	lock, ok, err := Acquire(name)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrAlreadyRunning
	}

	return lock, nil
}

// ErrAlreadyRunning возвращается, когда блокировка уже занята.
var ErrAlreadyRunning = errors.New("экземпляр уже запущен")
