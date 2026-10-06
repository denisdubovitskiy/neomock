// Package testsrc — фикстура source:test/all (§3.3, С4): интерфейс только в
// тестовом файле + обычный интерфейс в исходнике.
package testsrc

// Prod — интерфейс из GoFiles.
type Prod interface {
	Ping() error
}
