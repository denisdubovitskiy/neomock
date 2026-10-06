package pkg

import "context"

// Server — экспортированный интерфейс.
type Server interface {
	Get(ctx context.Context, id string) (string, error)
	Put(ctx context.Context, v string) error
}

// Private — интерфейс с приватными методами.
type Private interface {
	Get() string
	mustEmbed()
}

// Generic — generic-интерфейс.
type Generic[T any] interface {
	Get(ctx context.Context, id T) (T, error)
}

// Store — интерфейс с богатой сигнатурой для сквозных тестов эмиссии:
// []byte, variadic, void-метод, метод без контекста.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key string, val []byte, opts ...string) error
	Notify(msg string)
	Count(prefix string) int
}
