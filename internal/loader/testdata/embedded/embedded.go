// Package embedded — фикстуры встраивания интерфейсов для сквозных тестов
// (DESIGN §3.4, план этап 8, сценарий C5б).
package embedded

import (
	"context"
	"io"

	"github.com/denisdubovitskiy/neomock/internal/loader/testdata/pkg"
)

// ReaderWriter — встраивание двух интерфейсов стандартной библиотеки.
type ReaderWriter interface {
	io.Reader
	io.Writer
}

// D / A / B / Diamond — ромбовидное встраивание: Get приходит двумя путями,
// в моке остаётся один метод.
type D interface {
	Get() string
}

type A interface {
	D
}

type B interface {
	D
}

type Diamond interface {
	A
	B
}

// Greet — gRPC-подобный интерфейс: unexported-метод требует автовстраивания
// самого интерфейса в структуру мока (DESIGN §3.4).
type Greet interface {
	mustEmbedGreet()
	Hello(ctx context.Context, name string) (string, error)
}

// IntStore — встраивание инстанцированного generic-интерфейса чужого пакета:
// методы разворачиваются с подставленными аргументами типа.
type IntStore interface {
	pkg.Generic[int]
}

// secret — unexported-интерфейс: имя недоступно из чужих пакетов,
// мок без inpackage невозможен (негативная фикстура).
type secret interface {
	Get() string
}
