// Package config — конфигурация neomock: YAML/TOML-файл, go:generate-строка,
// слияние слоёв с трассировкой источника, валидация и explain (DESIGN §4).
//
// Файл и go:generate равноправны (§4.1); приоритет (слабый → сильный):
// builtins → defaults → for-package → for-interface → go:generate → CLI → env.
package config

// Version — единственная поддерживаемая версия схемы конфига (§4.2).
const Version = 1

// Файлы конфигурации, которые ищет обнаружение (§4.1).
var FileNames = []string{".neomock.yml", ".neomock.yaml", ".neomock.toml"}

// EnvConfig / EnvLogLevel — переменные окружения (§4.6).
const (
	EnvConfig   = "NEOMOCK_CONFIG"
	EnvLogLevel = "NEOMOCK_LOG_LEVEL"
)

// File — корень файла конфигурации (YAML и TOML 1:1, §4.2).
type File struct {
	// Version — версия схемы, обязана быть 1.
	Version int
	// Defaults — значения по умолчанию для всех пакетов.
	Defaults *Settings
	// Packages — секции пакетов по import path.
	Packages map[string]*PackageCfg
}

// PackageCfg — секция пакета: ровно один селектор + опциональный config (§4.2).
type PackageCfg struct {
	// All — генерировать все интерфейсы пакета.
	All bool
	// Include — regexp имён интерфейсов для включения.
	Include string
	// Exclude — regexp имён интерфейсов для исключения (только с include).
	Exclude string
	// Interfaces — точечные интерфейсы: имя → конфиг.
	Interfaces map[string]*IfaceCfg
	// Config — настройки уровня пакета.
	Config *Settings
}

// IfaceCfg — секция интерфейса: опциональный source-interface + config (§4.2).
type IfaceCfg struct {
	// SourceInterface — алиас: генерировать из другого интерфейса того же пакета.
	SourceInterface string
	// Config — настройки уровня интерфейса.
	Config *Settings
}

// Settings — все настройки генерации; nil-поле означает «не задано на этом
// слое» и заполняется из более слабого слоя при слиянии (§4.1).
type Settings struct {
	// Dir — каталог мока (шаблон).
	Dir *string
	// Filename — имя файла мока (шаблон).
	Filename *string
	// Pkgname — имя пакета мока.
	Pkgname *string
	// Structname — имя структуры мока (шаблон).
	Structname *string
	// Source — go|test|all (§3.3).
	Source *string
	// InPackage — мок в пакете источника (§3.3).
	InPackage *bool
	// BuildTags — теги сборки для загрузки источника.
	BuildTags *string
	// ForceWrite — перезаписывать файлы с ручными правками (§5).
	ForceWrite *bool
	// VerifyBuild — go build затронутых пакетов после записи (§4.2).
	VerifyBuild *bool
	// StrictCalls — строгий режим ошибок вызовов (§3.2.5).
	StrictCalls *bool
	// History — запись истории вызовов (§3.2.6).
	History *bool
	// GenerateLine — вставлять //go:generate-директиву в сгенерированный
	// файл (§4.2); false — директива не эмитится.
	GenerateLine *bool
	// Recursive — разрешает паттерн /... для своего модуля (§3.3).
	Recursive *bool
	// ExcludePackages — regexp путей пакетов-исключений при /... обходе.
	ExcludePackages []string
	// TemplateData — закрытая схема доп. данных (§4.2).
	TemplateData *TemplateData
}

// TemplateData — template-data: только известные ключи (схема закрытая, §4.2).
type TemplateData struct {
	// BoilerplateFile — файл с шапкой (лицензией), эмитится перед маркером.
	BoilerplateFile *string
}

// BuiltinDir — дефолтный шаблон dir; конвейер сверяет, переопределён ли он
// пользователем для внешних пакетов (§3.3).
func BuiltinDir() string { return "{{.InterfaceDir}}/mocks" }

// BuiltinDefaults — встроенные значения по умолчанию (§4.2 defaults).
func BuiltinDefaults() *Settings {
	s := func(v string) *string { return &v }
	b := func(v bool) *bool { return &v }
	return &Settings{
		Dir:          s("{{.InterfaceDir}}/mocks"),
		Filename:     s("{{.InterfaceSnake}}_mock.go"),
		Pkgname:      s("mocks"),
		Structname:   s("{{.Mock}}{{.InterfaceName}}"),
		Source:       s("go"),
		InPackage:    b(false),
		BuildTags:    s(""),
		ForceWrite:   b(true),
		VerifyBuild:  b(false),
		StrictCalls:  b(false),
		History:      b(true),
		GenerateLine: b(true),
		TemplateData: &TemplateData{BoilerplateFile: s("")},
	}
}
