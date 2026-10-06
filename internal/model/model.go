// Package model — внутреннее представление мокаемого интерфейса:
// Package/Interface/Method/Param/Result и конвертеры из go/types,
// включая плоское разворачивание встраивания (DESIGN 3.4).
package model

import (
	"fmt"

	"go/token"
	"go/types"
	"sort"
)

// Package — пакет-источник интерфейса.
type Package struct {
	// Path — import path пакета (ключ конфигурации).
	Path string
	// Name — имя пакета в исходниках.
	Name string
	// Dir — каталог пакета с исходниками (заполняет loader).
	Dir string
	// InModule — пакет внутри текущего модуля (иначе GOROOT/module cache).
	InModule bool
}

// Param — параметр метода с флагами для генерата.
type Param struct {
	// Name — имя из сигнатуры (может быть пустым — генерат подставит argN).
	Name string
	// Type — тип параметра из go/types.
	Type types.Type
	// IsSlice — статический underlying slice: история клонируется slices.Clone.
	IsSlice bool
	// IsMap — статический underlying map: история клонируется maps.Clone.
	IsMap bool
	// IsContext — точный context.Context: допускает wildcard neomock.Context.
	IsContext bool
}

// Result — результат метода.
type Result struct {
	// Name — имя из сигнатуры; пустое — генерат подставит (v/err/rN).
	Name string
	// Type — тип результата из go/types.
	Type types.Type
}

// Method — метод мокаемого интерфейса (плоский, после разворачивания).
type Method struct {
	// Name — имя метода.
	Name string
	// Sig — полная сигнатура (для сравнения конфликтов).
	Sig *types.Signature
	// Params — параметры в порядке объявления.
	Params []Param
	// Results — результаты в порядке объявления.
	Results []Result
	// Variadic — последний параметр variadic.
	Variadic bool
	// Owner — пакет-владелец сигнатуры (для сообщений о конфликтах).
	Owner *Package
}

// TypeParam — параметр типа generic-интерфейса.
type TypeParam struct {
	// Name — имя параметра (T).
	Name string
	// Constraint — ограничение (интерфейс) для рендера объявления.
	Constraint types.Type
}

// ClosureItem — интерфейс из транзитивного замыкания встраивания.
type ClosureItem struct {
	// PkgPath — import path пакета интерфейса.
	PkgPath string
	// PkgName — имя пакета интерфейса.
	PkgName string
	// Name — имя интерфейса.
	Name string
	// HasUnexported — интерфейс объявляет unexported-методы:
	// для чужого пакета требуется автовстраивание в структуру мока.
	HasUnexported bool
	// Methods — имена собственных (explicit) методов интерфейса: эмиттер
	// исключает unexported-имена из явных реализаций, если этот интерфейс
	// автовстраивается (они удовлетворяются промоушеном, §3.4).
	Methods []string
}

// Interface — мокаемый интерфейс, готовый к эмиссии.
type Interface struct {
	// Name — имя интерфейса (ключ конфигурации может отличаться из-за source-interface).
	Name string
	// Exported — имя экспортировано.
	Exported bool
	// Pkg — пакет-источник.
	Pkg Package
	// File — файл объявления (для {{.InterfaceFile}}).
	File string
	// Methods — плоский список методов после разворачивания встраивания.
	Methods []*Method
	// TypeParams — параметры типа (пусто для не-generic).
	TypeParams []TypeParam
	// Closure — транзитивное замыкание встраивания, включая корневой интерфейс.
	Closure []ClosureItem
	// IFace — go/types-представление интерфейса (для assertion и рендера).
	IFace *types.Interface
	// Named — именованный тип интерфейса (для рендера compile-time assertion
	// через registry.TypeString с корректной квалификацией пакета).
	Named *types.Named
}

// MethodNames возвращает имена методов — контракт names.MethodSet.
func (i *Interface) MethodNames() []string {
	out := make([]string, len(i.Methods))
	for k, m := range i.Methods {
		out[k] = m.Name
	}
	return out
}

// HasUnexportedMethods сообщает, объявляет ли интерфейс unexported-методы.
func (i *Interface) HasUnexportedMethods() bool {
	for _, m := range i.Methods {
		if !token.IsExported(m.Name) {
			return true
		}
	}
	return false
}

// FromNamed строит Interface из именованного типа интерфейса.
// Если тип не является интерфейсом — возвращает ошибку.
func FromNamed(named *types.Named) (*Interface, error) {
	underlying := named.Underlying()
	iface, ok := underlying.(*types.Interface)
	if !ok {
		return nil, errNotInterface{named.Obj().Name()}
	}

	pkg := named.Obj().Pkg()
	p := Package{Path: pkg.Path(), Name: pkg.Name(), InModule: pkg.Path() != ""}

	out := &Interface{
		Name:     named.Obj().Name(),
		Exported: token.IsExported(named.Obj().Name()),
		Pkg:      p,
		IFace:    iface,
		Named:    named,
	}
	for i := 0; i < named.TypeParams().Len(); i++ {
		tp := named.TypeParams().At(i)
		out.TypeParams = append(out.TypeParams, TypeParam{
			Name:       tp.Obj().Name(),
			Constraint: tp.Constraint(),
		})
	}

	if err := out.flatten(iface, &p, named); err != nil {
		return nil, err
	}
	return out, nil
}

// flatten заполняет Methods + Closure, отслеживая source-interface каждого метода.
// Методы хранятся в отсортированном по имени порядке: go/types сам отдаёт
// ExplicitMethod отсортированным, сортировка делается явной, чтобы порядок
// секций генерата был детерминированным (обязателен для skip-write-if-equal, §5).
func (i *Interface) flatten(
	iface *types.Interface,
	rootPkg *Package,
	rootNamed *types.Named,
) error {
	seen := map[string]*Method{}
	var order []*Method
	closureSeen := map[string]bool{}
	var errs []error

	record := func(named *types.Named) {
		pkg := named.Obj().Pkg()
		if pkg == nil {
			return // universe-типы (например, встроенный error) не дают импортов
		}
		key := pkg.Path() + "." + named.Obj().Name()
		if closureSeen[key] {
			return
		}
		closureSeen[key] = true
		innerIface, _ := named.Underlying().(*types.Interface)
		item := ClosureItem{
			PkgPath:       pkg.Path(),
			PkgName:       pkg.Name(),
			Name:          named.Obj().Name(),
			HasUnexported: innerIface != nil && hasUnexported(innerIface),
		}
		if innerIface != nil {
			for k := 0; k < innerIface.NumExplicitMethods(); k++ {
				item.Methods = append(item.Methods, innerIface.ExplicitMethod(k).Name())
			}
		}
		i.Closure = append(i.Closure, item)
	}

	var walk func(*types.Interface, *Package)
	walk = func(cur *types.Interface, owner *Package) {
		if rootNamed != nil {
			record(rootNamed)
			rootNamed = nil // записываем только один раз
		}
		for k := 0; k < cur.NumExplicitMethods(); k++ {
			f := cur.ExplicitMethod(k)
			if m := i.addMethod(f, owner, seen, &errs); m != nil {
				order = append(order, m)
			}
		}
		for k := 0; k < cur.NumEmbeddeds(); k++ {
			et := cur.EmbeddedType(k)
			u := types.Unalias(et)
			switch n := u.(type) {
			case *types.Named:
				record(n)
				if inner, ok := n.Underlying().(*types.Interface); ok {
					walk(inner, ownerOf(et))
				}
			case *types.Interface:
				walk(n, ownerOf(et))
			}
		}
	}
	walk(iface, rootPkg)

	if len(errs) > 0 {
		return errs[0]
	}
	sort.Slice(order, func(a, b int) bool { return order[a].Name < order[b].Name })
	i.Methods = order
	return nil
}

func hasUnexported(iface *types.Interface) bool {
	for k := 0; k < iface.NumExplicitMethods(); k++ {
		if !token.IsExported(iface.ExplicitMethod(k).Name()) {
			return true
		}
	}
	return false
}

func ownerOf(t types.Type) *Package {
	var named *types.Named
	switch v := t.(type) {
	case *types.Named:
		named = v
	case *types.Pointer:
		if e, ok := v.Elem().(*types.Named); ok {
			named = e
		}
	case *types.Map:
		if e, ok := v.Key().(*types.Named); ok {
			named = e
		}
	}
	if named == nil {
		return nil
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return nil
	}
	return &Package{Path: pkg.Path(), Name: pkg.Name()}
}

// addMethod добавляет метод, если он новый; дубликаты с идентичной сигнатурой
// (diamond) пропускаются, конфликт имён с разными сигнатурами — ошибка.
// Возвращает добавленный метод или nil для дубля/конфликта.
func (i *Interface) addMethod(
	f *types.Func,
	owner *Package,
	seen map[string]*Method,
	errs *[]error,
) *Method {
	sig := f.Type().(*types.Signature)
	m := &Method{
		Name:     f.Name(),
		Sig:      sig,
		Owner:    owner,
		Variadic: sig.Variadic(),
	}
	params := sig.Params()
	for k := 0; k < params.Len(); k++ {
		v := params.At(k)
		m.Params = append(m.Params, Param{
			Name:      v.Name(),
			Type:      v.Type(),
			IsSlice:   isSlice(v.Type()),
			IsMap:     isMap(v.Type()),
			IsContext: isContext(v.Type()),
		})
	}
	res := sig.Results()
	if res != nil {
		for k := 0; k < res.Len(); k++ {
			v := res.At(k)
			m.Results = append(m.Results, Result{
				Name: v.Name(),
				Type: v.Type(),
			})
		}
	}
	if prev, ok := seen[m.Name]; ok {
		if prev == m || types.Identical(prev.Sig, sig) {
			return nil
		}
		*errs = append(*errs, fmt.Errorf(
			"conflict method %s: %s vs %s",
			m.Name,
			ownerLabel(prev.Owner),
			ownerLabel(owner),
		))
		return nil
	}
	seen[m.Name] = m
	return m
}

// ownerLabel возвращает import path пакета-владельца для сообщений о конфликтах.
func ownerLabel(p *Package) string {
	if p == nil || p.Path == "" {
		return "<unknown>"
	}
	return p.Path
}

// isSlice сообщает о статическом slice-underlying — включая именованные
// типы вроде `type IDs []string`: история клонируется slices.Clone, который
// принимает ~[]E (DESIGN 3.2.5 п.4). any и типовые параметры — не slice.
func isSlice(t types.Type) bool {
	_, ok := types.Unalias(t).Underlying().(*types.Slice)
	return ok
}

// isMap — аналог isSlice для map-underlying (клонирование maps.Clone).
func isMap(t types.Type) bool {
	_, ok := types.Unalias(t).Underlying().(*types.Map)
	return ok
}

func isContext(t types.Type) bool {
	u := types.Unalias(t)
	n, ok := u.(*types.Named)
	if !ok {
		return false
	}
	// Universe-типы (error) не имеют пакета.
	pkg := n.Obj().Pkg()
	return pkg != nil && pkg.Path() == "context" && n.Obj().Name() == "Context"
}

// errNotInterface — ошибка: именованный тип не является интерфейсом
// и не может быть замоканирован.
type errNotInterface struct {
	// Name — имя типа, оказавшегося не интерфейсом.
	Name string
}

// Error возвращает сообщение о том, что тип не является интерфейсом.
func (e errNotInterface) Error() string {
	return fmt.Sprintf(
		"%s is not an interface",
		e.Name,
	)
}
