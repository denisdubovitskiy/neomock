// Package ping — пример TOML-конфига: мок в текущую директорию (inpackage).
package ping

// Pinger — контракт пинга.
type Pinger interface {
	// Ping возвращает имя узла.
	Ping() (string, error)
}
