// Package constraints — фикстура constraint-интерфейсов (§3.3): пропуск с
// предупреждением при селекции, ошибка при явном запросе.
package constraints

// Number — union-constraint: мокирование невозможно.
type Number interface {
	~int | ~string
}

// Comparable — constraint через встроенный comparable.
type Comparable interface {
	comparable
}

// Usable — обычный интерфейс рядом с constraints: должен оставаться в селекции.
type Usable interface {
	Do() error
}
