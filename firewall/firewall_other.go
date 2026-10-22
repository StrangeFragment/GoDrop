//go:build !windows

package firewall

import "errors"

// Sync доступна только в Windows: включение правила на других платформах
// возвращает понятную ошибку, выключение ничего не меняет.
func Sync(enabled bool, _ int) error {
	if !enabled {
		return nil
	}

	return errors.New("правила брандмауэра Windows поддерживаются только в Windows")
}
