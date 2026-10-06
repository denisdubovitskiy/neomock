// Package store — пример TOML-конфига: мок во вложенную директорию mocks/.
package store

import "context"

// Store — контракт хранилища.
type Store interface {
	// Get читает значение по ключу.
	Get(ctx context.Context, key string) (string, error)
}
