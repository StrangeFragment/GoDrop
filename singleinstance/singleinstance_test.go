package singleinstance

import (
	"fmt"
	"testing"
	"time"
)

func TestAcquireBlocksSecondCopy(t *testing.T) {
	t.Parallel()

	name := fmt.Sprintf("GoDropTest-%d", time.Now().UnixNano())

	first, ok, err := Acquire(name)
	if err != nil {
		t.Fatalf("Acquire() error: %v", err)
	}

	if !ok {
		t.Fatal("первая блокировка должна быть получена")
	}

	_, ok, err = Acquire(name)
	if err != nil {
		t.Fatalf("второй Acquire() error: %v", err)
	}

	if ok {
		t.Fatal("вторая блокировка не должна выдаваться")
	}

	first.Release()

	third, ok, err := Acquire(name)
	if err != nil {
		t.Fatalf("третий Acquire() error: %v", err)
	}

	if !ok {
		t.Fatal("после Release блокировка должна снова выдаваться")
	}

	third.Release()
	// Повторный Release не должен паниковать.
	third.Release()
}

func TestAcquireEmptyName(t *testing.T) {
	t.Parallel()

	if _, _, err := Acquire(""); err == nil {
		t.Fatal("ожидалась ошибка для пустого имени")
	}
}
