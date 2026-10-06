// Package cache — пример: мок генерируется в текущую директорию (inpackage).
package cache

import "context"

// refresher — приватный интерфейс: мок получает имя mockrefresher
// и конструктор newMockrefresher в этом же пакете.
type refresher interface {
	// Refresh обновляет кэш.
	Refresh(ctx context.Context) error
}
