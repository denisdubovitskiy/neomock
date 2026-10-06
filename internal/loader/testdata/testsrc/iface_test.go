package testsrc

// TestOnly — интерфейс только в *_test.go: доступен при source:test|all (§3.3).
type TestOnly interface {
	Fake() string
}
