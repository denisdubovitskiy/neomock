// Package directive — фикстура директивы //neomock:generate=false (§4.5).
package directive

// Active — обычный интерфейс.
type Active interface {
	Do() error
}

//neomock:generate=false
type Skipped interface {
	Never() error
}

//neomock:generate=false
type (
	// SkippedGroup — интерфейс в группе объявлений с директивой.
	SkippedGroup interface {
		NeverToo() error
	}
)
