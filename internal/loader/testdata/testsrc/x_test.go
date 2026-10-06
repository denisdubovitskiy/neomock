package testsrc_test

// External — интерфейс во внешнем тест-пакете (foo_test): доступен при
// source:test|all (§3.3).
type External interface {
	Remote() int
}
