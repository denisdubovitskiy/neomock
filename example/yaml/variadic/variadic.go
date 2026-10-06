// Package variadic — пример: интерфейсы с variadic-аргументами.
//
// Мок генерируется в текущую директорию в variadic_mock_test.go
// (секция packages в ../.neomock.yml). Настройка ожиданий — variadic_test.go.
package variadic

import "context"

// Tag — именованный тип: variadic работает и поверх него.
type Tag string

// Notifier — отправитель уведомлений: каждый метод закрывает
// свою форму variadic-сигнатуры.
type Notifier interface {
	// Send — фиксированные аргументы + variadic-список получателей.
	Send(ctx context.Context, msg string, to ...string) error
	// Broadcast — variadic как единственный аргумент, без результатов.
	Broadcast(msgs ...string)
	// Count — variadic из именованного типа, с результатом.
	Count(tags ...Tag) int
}
