// Package queue — пример: мок генерируется в текущую директорию
// в файл queue_mock_test.go (не попадает в прод-сборку).
package queue

import "context"

// Queue — очередь сообщений.
type Queue interface {
	// Push добавляет сообщение.
	Push(ctx context.Context, msg string) error
	// Pop извлекает сообщение.
	Pop(ctx context.Context) (string, error)
}
