// Package store — пример: мок генерируется во вложенную директорию mocks/.
package store

import "context"

// Store — контракт хранилища.
type Store interface {
	// Get читает значение по ключу.
	Get(ctx context.Context, key string) (string, error)
	// Put записывает значение по ключу.
	Put(ctx context.Context, key, value string) error
}
