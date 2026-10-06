// Package testsrc — пример: интерфейс объявлен в _test.go (source: test).
package testsrc

// TestOnly — интерфейс из тестового файла: виден только при source: test|all,
// мок генерируется в _test.go этого же пакета.
type TestOnly interface {
	// Ping проверяет доступность.
	Ping() error
}
