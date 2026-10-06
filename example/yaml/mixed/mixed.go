// Package mixed — пример source: all: интерфейс из исходника.
package mixed

// FromGo — интерфейс из обычного исходного файла.
type FromGo interface {
	// Do выполняет действие.
	Do() error
}
