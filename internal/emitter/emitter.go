// Package emitter генерирует исходный код мока по model.Interface:
// типизированный DSL When/Then/Times/Once/Optional/Never + Run-fallback
// с кардинальностью, историей вызовов и Verify (DESIGN §3.2.1, §3.4,
// Приложение А). Скелет кода — text/template (mock.tmpl), все типы
// рендерятся через Registry с учётом импортов.
package emitter

import (
	"embed"
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"text/template"
	"unicode"

	"github.com/denisdubovitskiy/neomock/internal/model"
	"github.com/denisdubovitskiy/neomock/internal/names"
	"github.com/denisdubovitskiy/neomock/internal/registry"
)

//go:embed mock.tmpl
var tmplFS embed.FS

var mockTmpl = template.Must(template.ParseFS(tmplFS, "mock.tmpl"))

// neomockPath — import path рантайма, импортируемого каждым моком.
const neomockPath = "github.com/denisdubovitskiy/neomock"

// universeError — тип error из вселенной go/types (для ErrorWrap).
var universeError = types.Universe.Lookup("error").Type()

// Options — параметры генерации одного мока.
type Options struct {
	// Package — имя пакета мока.
	Package string
	// SelfPath — import path пакета мока: типы этого пакета рендерятся
	// без квалификатора и без импорта (inpackage). Пусто — мок вне
	// пакета источника.
	SelfPath string
	// StructName — имя структуры мока; пусто — "Mock"+InterfaceName.
	StructName string
	// OutPath — путь файла мока для //go:generate-шапки; пусто — строка
	// не эмитится.
	OutPath string
	// GenerateLine — точная команда перегенерации (файловый режим:
	// `go tool neomock generate <pkg>`); задаётся конвейером, перекрывает
	// шаблон с OutPath.
	GenerateLine string
	// StrictCalls — базовое значение strict-calls из конфигурации
	// (печатаюся в конструктор; опция WithStrictCalls в коде приоритетнее).
	StrictCalls bool
	// DisableHistory отключает запись истории и методы <M>Calls /
	// <M>CallsCount (DESIGN 3.2.6, 3.2.5 п.5).
	DisableHistory bool
}

// fileData — корневые данные шаблона.
type fileData struct {
	GenerateLine string
	Package      string
	Imports      []registry.Import
	StructName   string
	CtorName     string
	TypeParams   string // "[T any]" или ""
	TypeArgs     string // "[T]" или ""
	StrictBase   bool
	History      bool
	Embeds       []string
	Methods      []*methodData
	Assertion    string // "pkg.Server" — квалифицированное имя интерфейса
}

// methodData — данные одного метода мока.
type methodData struct {
	Name        string
	MethodName  string // "MockServer.Get" — для сообщений
	WhenName    string
	RunName     string
	NeverName   string
	Holder      string
	HolderField string
	CallType    string
	ParamsType  string
	ResultsType string
	TypeParams  string
	TypeArgs    string
	History     bool
	Void        bool
	ErrorWrap   bool // последний результат — error: оборачивается в soft-режиме

	// Сигнатуры (готовые строки).
	SigParams   string // When/call/Run: "ctx context.Context, opts ...string"
	MatchParams string // match/diffArgs:     "ctx context.Context, opts []string"
	PassArgs    string // прямой вызов:        "ctx, id, opts..."
	MatchArgs   string // вызов match:         "ctx, id, opts"
	ResultsSig  string // "string, error" декорации: "" | "error" | "(string, error)"
	RunFnType   string // "func(ctx context.Context, id string) (string, error)"
	ThenParams  string // "v1 string, err error"
	ThenSet     string // "v1: v1, err: err"

	FormatArgsCall string // neomock.FormatArgs([]string{...}, []any{...})
	HistoryFields  string // "Ctx: ctx, Buf: slices.Clone(buf)"

	ServedReturns string // "_res.v1, _res.err"
	ZeroReturns   string // "v1, err"
	WrapField     string // поле последнего error-результата для ErrUnexpectedCall

	Params  []paramData
	Results []resultData
}

// paramData — данные одного параметра метода.
type paramData struct {
	// Name — имя в сигнатуре (argN для безымянных, с дедупликацией).
	Name string
	// Field — имя поля expectation (дедупликация против служебных полей).
	Field string
	// Exported — имя поля Params-структуры истории (экспортированное).
	Exported string
	// Type — тип параметра; для variadic — тип среза ([]PutOption).
	Type string
	// SigType — тип в сигнатуре: "...PutOption" для variadic, иначе Type.
	SigType  string
	Variadic bool
	// Clone — "slices.Clone"/"maps.Clone" для статических slice/map
	// underlying при записи истории; "" — пишется как есть.
	Clone string
	// Index — позиция параметра (для arg[i] в mismatch-репортах).
	Index int
}

// resultData — данные одного результата метода.
type resultData struct {
	// Field — имя поля Results-структуры и параметра Then.
	Field string
	// Type — тип результата.
	Type string
}

// Render генерирует код мока интерфейса. Возвращает неотформатированный
// исходник; форматирование — internal/format.Source.
func Render(iface *model.Interface, opts Options) (string, error) {
	if opts.Package == "" {
		return "", fmt.Errorf("emitter: имя пакета мока обязательно")
	}
	structName := opts.StructName
	if structName == "" {
		structName = "Mock" + iface.Name
	}
	history := !opts.DisableHistory

	// Fail-fast коллизии служебных имён с методами интерфейса (DESIGN 3.2.6).
	if err := names.Validate(iface, history); err != nil {
		return "", fmt.Errorf("emitter: %w", err)
	}

	reg := registry.New(opts.SelfPath)
	d := &fileData{
		Package:    opts.Package,
		StructName: structName,
		CtorName:   ctorName(structName),
		StrictBase: opts.StrictCalls,
		History:    history,
	}

	tp, ta, err := buildTypeParams(reg, iface)
	if err != nil {
		return "", err
	}
	d.TypeParams, d.TypeArgs = tp, ta

	// Автовстраивание чужих интерфейсов с unexported-методами (DESIGN §3.4):
	// свой пакет (inpackage) покрывается явными реализациями.
	// Unexported-методы автовстраиваемых интерфейсов явных реализаций НЕ
	// получают: они удовлетворяются промоушеном от встроенного интерфейса
	// (явная реализация дала бы коллизию «field and method with the same
	// name» с полем-холдером). Экспортированные методы — всегда явно.
	embedCovered := map[string]bool{}
	for _, c := range iface.Closure {
		if !c.HasUnexported || c.PkgPath == opts.SelfPath {
			continue
		}
		alias, err := reg.Register(c.PkgPath, c.PkgName)
		if err != nil {
			return "", err
		}
		d.Embeds = append(d.Embeds, alias+"."+c.Name)
		for _, name := range c.Methods {
			if !token.IsExported(name) {
				embedCovered[name] = true
			}
		}
	}

	for _, m := range iface.Methods {
		if embedCovered[m.Name] {
			continue
		}
		md, err := buildMethod(reg, m, structName, tp, ta, history)
		if err != nil {
			return "", fmt.Errorf("emitter: метод %s: %w", m.Name, err)
		}
		d.Methods = append(d.Methods, md)
	}

	// Compile-time assertion: квалифицированное имя интерфейса через
	// Registry (регистрирует импорт пакета источника — DESIGN §3.4).
	// Для generic-мока assertion невозможна (нужна конкретная инстанциация),
	// и регистрация пакета источника не выполняется — импорт может оказаться
	// неиспользуемым.
	if iface.Named != nil && tp == "" {
		a, err := reg.TypeString(iface.Named)
		if err != nil {
			return "", fmt.Errorf("emitter: assertion: %w", err)
		}
		d.Assertion = a
	}

	// Постоянные импорты: fmt — только при наличии методов (сообщения),
	// sync — мьютексы и verifyOnce, neomock — рантайм.
	if len(d.Methods) > 0 {
		if _, err := reg.Register("fmt", "fmt"); err != nil {
			return "", err
		}
	}
	if _, err := reg.Register("sync", "sync"); err != nil {
		return "", err
	}
	if _, err := reg.Register(neomockPath, "neomock"); err != nil {
		return "", err
	}
	// slices/maps — только при клонировании истории (DESIGN 3.2.5 п.4).
	for _, md := range d.Methods {
		for _, p := range md.Params {
			if p.Clone == "" {
				continue
			}
			if _, err := reg.Register(strings.TrimSuffix(p.Clone, ".Clone"), strings.TrimSuffix(p.Clone, ".Clone")); err != nil {
				return "", err
			}
		}
	}
	d.Imports = reg.Imports()

	switch {
	case opts.GenerateLine != "":
		d.GenerateLine = opts.GenerateLine
	case opts.OutPath != "":
		d.GenerateLine = fmt.Sprintf(
			"//go:generate go tool neomock --interface %s --out %s --pkgname %s",
			iface.Name,
			opts.OutPath,
			opts.Package,
		)
		if opts.StructName != "" && opts.StructName != "Mock"+iface.Name {
			d.GenerateLine += " --structname " + opts.StructName
		}
	}

	var b strings.Builder
	if err := mockTmpl.Execute(&b, d); err != nil {
		return "", fmt.Errorf("emitter: template: %w", err)
	}
	return b.String(), nil
}

// buildTypeParams рендерит объявления и аргументы type-параметров мока.
func buildTypeParams(reg *registry.Registry, iface *model.Interface) (string, string, error) {
	if len(iface.TypeParams) == 0 {
		return "", "", nil
	}
	var decls, args []string
	for _, tparam := range iface.TypeParams {
		c, err := reg.TypeString(tparam.Constraint)
		if err != nil {
			return "", "", fmt.Errorf("emitter: typeparam %s: %w", tparam.Name, err)
		}
		decls = append(decls, tparam.Name+" "+c)
		args = append(args, tparam.Name)
	}
	return "[" + strings.Join(decls, ", ") + "]", "[" + strings.Join(args, ", ") + "]", nil
}

// buildMethod собирает данные шаблона одного метода. Все TypeString-вызовы
// регистрируют импорты в reg — порядок методов влияет только на то, какой
// пакет получит базовое имя, не на корректность.
func buildMethod(
	reg *registry.Registry,
	m *model.Method,
	structName,
	typeParams,
	typeArgs string,
	history bool,
) (*methodData, error) {
	md := &methodData{
		Name:        m.Name,
		MethodName:  structName + "." + m.Name,
		WhenName:    names.When(m.Name),
		RunName:     names.Run(m.Name),
		NeverName:   names.Never(m.Name),
		Holder:      names.Holder(structName, m.Name),
		HolderField: names.HolderField(m.Name),
		CallType:    names.CallType(structName, m.Name),
		ParamsType:  names.ParamsType(structName, m.Name),
		TypeParams:  typeParams,
		TypeArgs:    typeArgs,
		History:     history,
		Void:        len(m.Results) == 0,
	}
	if !md.Void {
		md.ResultsType = md.Holder + "Results"
	}

	// Параметры: имена сигнатуры, поля expectation/Params, типы.
	sigTaken := map[string]bool{}
	expTaken := map[string]bool{}
	for _, r := range []string{
		"results",
		"hasResults",
		"calls",
		"min",
		"max",
		"origin",
	} {
		expTaken[r] = true
	}
	paramsTaken := map[string]bool{}

	var sig, matchSig, pass, marg []string
	for i := range m.Params {
		p := &m.Params[i]
		variadic := m.Variadic && i == len(m.Params)-1

		name := dedupe(sigTaken, names.ParamName(i, p.Name))
		typeStr, err := reg.TypeString(p.Type)
		if err != nil {
			return nil, fmt.Errorf("параметр %s: %w", name, err)
		}
		sigType := typeStr
		if variadic {
			sigType, err = reg.ParamString(p.Type, true)
			if err != nil {
				return nil, fmt.Errorf("параметр %s: %w", name, err)
			}
		}

		pd := paramData{
			Name:     name,
			Field:    dedupe(expTaken, name),
			Exported: names.Field(paramsTaken, name),
			Type:     typeStr,
			SigType:  sigType,
			Variadic: variadic,
			Index:    i,
		}
		// Клон истории — только статические slice/map underlying
		// (any и типовые параметры пишутся как есть, DESIGN 3.2.5 п.4).
		if history {
			switch {
			case p.IsMap:
				pd.Clone = "maps.Clone"
			case p.IsSlice:
				pd.Clone = "slices.Clone"
			}
		}
		md.Params = append(md.Params, pd)

		sig = append(sig, name+" "+sigType)
		matchSig = append(matchSig, name+" "+typeStr)
		if variadic {
			pass = append(pass, name+"...")
		} else {
			pass = append(pass, name)
		}
		marg = append(marg, name)
	}
	md.SigParams = strings.Join(sig, ", ")
	md.MatchParams = strings.Join(matchSig, ", ")
	md.PassArgs = strings.Join(pass, ", ")
	md.MatchArgs = strings.Join(marg, ", ")

	// История: поля записи с клонированием.
	var hist []string
	for _, pd := range md.Params {
		if pd.Clone != "" {
			hist = append(hist, fmt.Sprintf(
				"%s: %s(%s)",
				pd.Exported,
				pd.Clone,
				pd.Name,
			))
		} else {
			hist = append(hist, fmt.Sprintf("%s: %s", pd.Exported, pd.Name))
		}
	}
	md.HistoryFields = strings.Join(hist, ", ")

	// Сообщения: neomock.FormatArgs([]string{...}, []any{...}).
	quoted := make([]string, len(md.Params))
	for i, pd := range md.Params {
		quoted[i] = fmt.Sprintf("%q", pd.Name)
	}
	md.FormatArgsCall = fmt.Sprintf("neomock.FormatArgs([]string{%s}, []any{%s})",
		strings.Join(quoted, ", "), md.MatchArgs)

	// Результаты: поля Results-структуры = параметры Then.
	resTaken := map[string]bool{}
	var thenParams, thenSet, zeroRet, servedRet []string
	for i := range m.Results {
		r := &m.Results[i]
		base := r.Name
		if base == "" || base == "_" {
			base = fmt.Sprintf("v%d", i+1)
		}
		field := dedupe(resTaken, base)
		typeStr, err := reg.TypeString(r.Type)
		if err != nil {
			return nil, fmt.Errorf("результат %s: %w", field, err)
		}
		md.Results = append(md.Results, resultData{Field: field, Type: typeStr})
		thenParams = append(thenParams, field+" "+typeStr)
		thenSet = append(thenSet, field+": "+field)
		zeroRet = append(zeroRet, field)
		servedRet = append(servedRet, "_res."+field)
	}
	md.ThenParams = strings.Join(thenParams, ", ")
	md.ThenSet = strings.Join(thenSet, ", ")
	md.ZeroReturns = strings.Join(zeroRet, ", ")
	md.ServedReturns = strings.Join(servedRet, ", ")
	if len(md.Results) > 0 && types.Identical(m.Results[len(m.Results)-1].Type, universeError) {
		md.ErrorWrap = true
		md.WrapField = md.Results[len(md.Results)-1].Field
	}

	// Сигнатура метода: "" | "error" | "(string, error)".
	var resTypes []string
	for _, rd := range md.Results {
		resTypes = append(resTypes, rd.Type)
	}
	switch len(resTypes) {
	case 0:
		md.ResultsSig = ""
	case 1:
		md.ResultsSig = resTypes[0]
	default:
		md.ResultsSig = "(" + strings.Join(resTypes, ", ") + ")"
	}

	// Тип fallback-функции Run: в точности сигнатура метода, с именами
	// параметров (DESIGN §3.2.3).
	md.RunFnType = "func(" + md.SigParams + ")"
	if md.ResultsSig != "" {
		md.RunFnType += " " + md.ResultsSig
	}

	return md, nil
}

// dedupe возвращает base или base_1, base_2, ... — первое свободное имя.
func dedupe(taken map[string]bool, base string) string {
	if base == "" {
		base = "v"
	}
	if !taken[base] {
		taken[base] = true
		return base
	}
	for i := 1; ; i++ {
		c := fmt.Sprintf("%s_%d", base, i)
		if !taken[c] {
			taken[c] = true
			return c
		}
	}
}

// ctorName вычисляет имя конструктора: New+Имя для экспортированной
// структуры, new+Имя (с заглавной) для приватной (DESIGN 3.3: mockfoo →
// newMockfoo).
func ctorName(structName string) string {
	r := []rune(structName)
	if len(r) > 0 && !unicode.IsUpper(r[0]) {
		return "new" + string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	return "New" + structName
}
