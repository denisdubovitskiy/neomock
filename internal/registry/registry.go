// Package registry ведёт учёт импортов сгенерированного файла: назначает
// алиасы с дедупликацией (template, template0, ...), учитывает self-пакет
// (без импорта) и рекурсивно обходит дерево types.Type, включая TypeArgs
// инстанциаций (DESIGN 3.4).
package registry

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
)

// Import — назначенный импорт сгенерированного файла.
type Import struct {
	// Alias — имя, под которым пакет импортируется в файле.
	Alias string
	// Path — import path пакета.
	Path string
}

// Registry распределяет алиасы импортов для одного генерируемого файла.
type Registry struct {
	// self — import path пакета, в который пишется мок: его типы
	// рендерятся без квалификатора и без импорта.
	self string
	// byPath — назначенный алиас по import path.
	byPath map[string]string
	// used — занятые алиасы.
	used map[string]bool
}

// New создаёт Registry для пакета мока self (может быть пустым).
func New(self string) *Registry {
	return &Registry{
		self:   self,
		byPath: map[string]string{},
		used:   map[string]bool{},
	}
}

// lastSegment извлекает последний сегмент import path как имя пакета.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// Register возвращает алиас для пакета path. Повторный вызов с тем же path
// даёт тот же алиас. Первый претендент — имя пакета (или последний сегмент
// path при пустом name), конфликты получают суффикс 0, 1, ...
// Пустой path (universe-типы: int, string, error) — пустой алиас.
func (r *Registry) Register(path, name string) (string, error) {
	if path == "" {
		return "", nil
	}
	if r.self != "" && path == r.self {
		// Типы своего пакета — без квалификатора.
		return "", nil
	}
	if a, ok := r.byPath[path]; ok {
		return a, nil
	}
	base := name
	if base == "" {
		base = lastSegment(path)
	}
	alias := base
	if r.used[alias] {
		for i := 0; ; i++ {
			candidate := fmt.Sprintf("%s%d", base, i)
			if !r.used[candidate] {
				alias = candidate
				break
			}
		}
	}
	r.used[alias] = true
	r.byPath[path] = alias
	return alias, nil
}

// Imports возвращает назначенные импорты, отсортированные по path —
// детерминированный порядок блока import.
func (r *Registry) Imports() []Import {
	out := make([]Import, 0, len(r.byPath))
	for p, a := range r.byPath {
		out = append(out, Import{Alias: a, Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// TypeString рендерит тип types.Type с квалификацией через Registry:
// регистрирует пакеты всех встреченных именованных типов (включая
// TypeArgs) и возвращает выражение, пригодное для генерата.
func (r *Registry) TypeString(t types.Type) (string, error) {
	var b strings.Builder
	if err := r.writeType(&b, t); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ParamString рендерит объявление параметра сигнатуры: обычный тип или
// "...elem" для variadic-позиции (срез последнего параметра разворачивается).
func (r *Registry) ParamString(t types.Type, variadic bool) (string, error) {
	if !variadic {
		return r.TypeString(t)
	}
	s, ok := types.Unalias(t).(*types.Slice)
	if !ok {
		return "", fmt.Errorf("registry: variadic param %s is not a slice", t)
	}
	var b strings.Builder
	b.WriteString("...")
	if err := r.writeType(&b, s.Elem()); err != nil {
		return "", err
	}
	return b.String(), nil
}

// writeType рекурсивно записывает тип, регистрируя импорты.
func (r *Registry) writeType(b *strings.Builder, t types.Type) error {
	switch t := t.(type) {
	case *types.Basic:
		b.WriteString(t.Name())
		return nil
	case *types.Named:
		obj := t.Obj()
		return r.writeQualified(b, obj.Pkg(), obj.Name(), t.TypeArgs())
	case *types.Alias:
		obj := t.Obj()
		return r.writeQualified(b, obj.Pkg(), obj.Name(), t.TypeArgs())
	case *types.TypeParam:
		b.WriteString(t.Obj().Name())
		return nil
	case *types.Pointer:
		b.WriteByte('*')
		return r.writeType(b, t.Elem())
	case *types.Slice:
		b.WriteString("[]")
		return r.writeType(b, t.Elem())
	case *types.Array:
		fmt.Fprintf(b, "[%d]", t.Len())
		return r.writeType(b, t.Elem())
	case *types.Map:
		b.WriteString("map[")
		if err := r.writeType(b, t.Key()); err != nil {
			return err
		}
		b.WriteByte(']')
		return r.writeType(b, t.Elem())
	case *types.Chan:
		switch t.Dir() {
		case types.SendOnly:
			b.WriteString("chan<- ")
		case types.RecvOnly:
			b.WriteString("<-chan ")
		default:
			b.WriteString("chan ")
		}
		return r.writeType(b, t.Elem())
	case *types.Struct:
		return r.writeStruct(b, t)
	case *types.Interface:
		if t.Empty() {
			// interface{} / any — рендерится в any (Go 1.18+)
			b.WriteString("any")
			return nil
		}
		return r.writeIfaceLiteral(b, t)
	case *types.Signature:
		return r.writeSignature(b, t)
	case *types.Tuple:
		return r.writeTuple(b, t)
	default:
		return fmt.Errorf("registry: unsupported type %T", t)
	}
}

// writeQualified записывает именованный тип с квалификатором пакета.
func (r *Registry) writeQualified(
	b *strings.Builder,
	pkg *types.Package,
	name string,
	targs *types.TypeList,
) error {
	alias := ""
	if pkg != nil {
		var err error
		alias, err = r.Register(pkg.Path(), pkg.Name())
		if err != nil {
			return err
		}
	}
	if alias != "" {
		b.WriteString(alias)
		b.WriteByte('.')
	}
	b.WriteString(name)
	return r.writeTypeArgs(b, targs)
}

// writeTypeArgs записывает аргументы типа инстанциации [A, B].
func (r *Registry) writeTypeArgs(b *strings.Builder, targs *types.TypeList) error {
	if targs == nil || targs.Len() == 0 {
		return nil
	}
	b.WriteByte('[')
	for i := 0; i < targs.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := r.writeType(b, targs.At(i)); err != nil {
			return err
		}
	}
	b.WriteByte(']')
	return nil
}

// writeStruct записывает тип структуры с тегами полей.
func (r *Registry) writeStruct(b *strings.Builder, t *types.Struct) error {
	b.WriteString("struct{")
	for i := 0; i < t.NumFields(); i++ {
		if i > 0 {
			b.WriteString("; ")
		}
		f := t.Field(i)
		if !f.Exported() {
			b.WriteByte('_')
		}
		b.WriteString(f.Name())
		b.WriteByte(' ')
		if err := r.writeType(b, f.Type()); err != nil {
			return err
		}
		if tag := t.Tag(i); tag != "" {
			fmt.Fprintf(b, " `%s`", tag)
		}
	}
	b.WriteByte('}')
	return nil
}

// writeIfaceLiteral записывает анонимный не-пустой интерфейс: явные методы
// и встроенные типы. Нужен для сигнатур вида `interface{ Foo() int }`.
func (r *Registry) writeIfaceLiteral(b *strings.Builder, t *types.Interface) error {
	b.WriteString("interface{")
	sep := ""
	for i := 0; i < t.NumExplicitMethods(); i++ {
		m := t.ExplicitMethod(i)
		b.WriteString(sep)
		sep = "; "
		b.WriteString(m.Name())
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return fmt.Errorf(
				"registry: interface method %s is not a signature",
				m.Name(),
			)
		}
		// В литерале интерфейса метод пишется без ключевого слова func.
		if err := r.writeSignatureBody(b, sig); err != nil {
			return err
		}
	}
	for i := 0; i < t.NumEmbeddeds(); i++ {
		b.WriteString(sep)
		sep = "; "
		if err := r.writeType(b, t.EmbeddedType(i)); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// writeSignature записывает func-тип: параметры, результаты, variadic.
func (r *Registry) writeSignature(b *strings.Builder, t *types.Signature) error {
	b.WriteString("func")
	return r.writeSignatureBody(b, t)
}

// writeSignatureBody записывает "(params) results" без префикса func.
func (r *Registry) writeSignatureBody(b *strings.Builder, t *types.Signature) error {
	b.WriteByte('(')
	params := t.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		pt := params.At(i).Type()
		if t.Variadic() && i == params.Len()-1 {
			b.WriteString("...")
			if s, ok := pt.(*types.Slice); ok {
				pt = s.Elem()
			}
		}
		if err := r.writeType(b, pt); err != nil {
			return err
		}
	}
	b.WriteByte(')')
	results := t.Results()
	switch {
	case results.Len() == 0:
	case results.Len() == 1:
		b.WriteByte(' ')
		return r.writeType(b, results.At(0).Type())
	default:
		b.WriteString(" (")
		for i := 0; i < results.Len(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := r.writeType(b, results.At(i).Type()); err != nil {
				return err
			}
		}
		b.WriteByte(')')
	}
	return nil
}

// writeTuple записывает кортеж типов в скобках.
func (r *Registry) writeTuple(b *strings.Builder, t *types.Tuple) error {
	b.WriteByte('(')
	for i := 0; i < t.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := r.writeType(b, t.At(i).Type()); err != nil {
			return err
		}
	}
	b.WriteByte(')')
	return nil
}
