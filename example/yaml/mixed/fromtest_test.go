package mixed

// FromTest — интерфейс из _test.go; при source: all виден наравне с FromGo.
type FromTest interface {
	// DoTest выполняет тестовое действие.
	DoTest() error
}
