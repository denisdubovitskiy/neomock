package config

import (
	"fmt"
	"strings"
)

// Layer — один слой слияния: имя для трассировки (explain, §4.7) и настройки.
type Layer struct {
	// Name — имя слоя: "defaults", "for-package <path>", "for-interface <name>",
	// "go:generate", "cli", "env".
	Name string
	// S — настройки слоя; nil-поля означают «слой не задаёт».
	S *Settings
}

// Resolved — итоговое значение настроек одного интерфейса с трассировкой
// источника каждого поля (§4.1, §4.7).
type Resolved struct {
	// Dir — каталог назначения мока (относительно корня модуля).
	Dir string
	// Filename — имя файла мока; для source:test оканчивается на _test.go.
	Filename string
	// Pkgname — имя пакета мока; пусто — пакет источника (inpackage).
	Pkgname string
	// Structname — имя структуры-мока, результат шаблона structname.
	Structname string
	// Source — вариант пакета для загрузки: go|test|all (§3.3).
	Source string
	// InPackage — мок генерируется в пакет источника.
	InPackage bool
	// BuildTags — теги сборки для go/packages (список через пробел).
	BuildTags string
	// ForceWrite — перезаписывать файл даже при совпадении содержимого.
	ForceWrite bool
	// VerifyBuild — собирать пакет мока после генерации (verify-build).
	VerifyBuild bool
	// StrictCalls — запрещать вызовы мока без настроенного ожидания.
	StrictCalls bool
	// History — генерировать историю вызовов (XxxCalls).
	History bool
	// GenerateLine — вставлять //go:generate-директиву в сгенерированный файл.
	GenerateLine bool
	// BoilerplateFile — файл с дополнительным заголовком сгенерированного кода.
	BoilerplateFile string
	// Sources — поле → имя слоя, задавшего значение (для explain).
	Sources map[string]string
}

// Resolve применяет слои от слабого к сильному (§4.1):
// builtins → defaults → for-package → for-interface → go:generate → CLI → env.
func Resolve(layers ...Layer) Resolved {
	r := Resolved{Sources: map[string]string{}}
	apply := func(layerName string, s *Settings) {
		if s == nil {
			return
		}
		setStr(&r.Dir, "dir", s.Dir, layerName, r.Sources)
		setStr(&r.Filename, "filename", s.Filename, layerName, r.Sources)
		setStr(&r.Pkgname, "pkgname", s.Pkgname, layerName, r.Sources)
		setStr(&r.Structname, "structname", s.Structname, layerName, r.Sources)
		setStr(&r.Source, "source", s.Source, layerName, r.Sources)
		setStr(&r.BuildTags, "build-tags", s.BuildTags, layerName, r.Sources)
		setBool(&r.InPackage, "inpackage", s.InPackage, layerName, r.Sources)
		setBool(&r.ForceWrite, "force-write", s.ForceWrite, layerName, r.Sources)
		setBool(&r.VerifyBuild, "verify-build", s.VerifyBuild, layerName, r.Sources)
		setBool(&r.StrictCalls, "strict-calls", s.StrictCalls, layerName, r.Sources)
		setBool(&r.History, "history", s.History, layerName, r.Sources)
		setBool(&r.GenerateLine, "generate-line", s.GenerateLine, layerName, r.Sources)
		if s.TemplateData != nil && s.TemplateData.BoilerplateFile != nil {
			r.BoilerplateFile = *s.TemplateData.BoilerplateFile
			r.Sources["boilerplate-file"] = layerName
		}
	}
	apply("builtins", BuiltinDefaults())
	for _, l := range layers {
		apply(l.Name, l.S)
	}
	return r
}

func setStr(
	dst *string,
	key string,
	v *string,
	layer string,
	src map[string]string,
) {
	if v != nil {
		*dst = *v
		src[key] = layer
	}
}

func setBool(
	dst *bool,
	key string,
	v *bool,
	layer string,
	src map[string]string,
) {
	if v != nil {
		*dst = *v
		src[key] = layer
	}
}

// ExplainForm — порядок и подписи полей в выводе explain (§4.7).
var ExplainForm = []struct{ Key, Label string }{
	{"dir", "dir"},
	{"filename", "filename"},
	{"pkgname", "pkgname"},
	{"structname", "structname"},
	{"source", "source"},
	{"inpackage", "inpackage"},
	{"strict-calls", "strict-calls"},
	{"history", "history"},
	{"generate-line", "generate-line"},
}

// Explain рендерит вывод команды explain: значение + источник (§4.7).
func (r *Resolved) Explain(ifaceID string) string {
	values := map[string]string{
		"dir":           r.Dir,
		"filename":      r.Filename,
		"pkgname":       r.Pkgname,
		"structname":    r.Structname,
		"source":        r.Source,
		"inpackage":     fmt.Sprintf("%v", r.InPackage),
		"strict-calls":  fmt.Sprintf("%v", r.StrictCalls),
		"history":       fmt.Sprintf("%v", r.History),
		"generate-line": fmt.Sprintf("%v", r.GenerateLine),
	}
	var b strings.Builder
	fmt.Fprintf(&b, "interface: %s\n", ifaceID)
	keys := make([]string, 0, len(ExplainForm))
	for _, f := range ExplainForm {
		keys = append(keys, f.Key)
	}
	width := 0
	for _, k := range keys {
		if len(k)+1 > width {
			width = len(k) + 1
		}
	}
	for _, f := range ExplainForm {
		label := f.Label + ":"
		src := r.Sources[f.Key]
		if src == "" {
			src = "builtins"
		}
		fmt.Fprintf(&b, "  %-*s %-*s (%s)\n", width, label, 24, values[f.Key], src)
	}
	return b.String()
}

// MergeSettings объединяет два блока настроек (для go:generate поверх файла):
// значения из overlay перекрывают base. Используется в тестах и CLI.
func MergeSettings(base, overlay *Settings) *Settings {
	out := *base
	if overlay == nil {
		return &out
	}
	pickStr := func(dst **string, v *string) {
		if v != nil {
			*dst = v
		}
	}
	pickBool := func(dst **bool, v *bool) {
		if v != nil {
			*dst = v
		}
	}
	pickStr(&out.Dir, overlay.Dir)
	pickStr(&out.Filename, overlay.Filename)
	pickStr(&out.Pkgname, overlay.Pkgname)
	pickStr(&out.Structname, overlay.Structname)
	pickStr(&out.Source, overlay.Source)
	pickStr(&out.BuildTags, overlay.BuildTags)
	pickBool(&out.InPackage, overlay.InPackage)
	pickBool(&out.ForceWrite, overlay.ForceWrite)
	pickBool(&out.VerifyBuild, overlay.VerifyBuild)
	pickBool(&out.StrictCalls, overlay.StrictCalls)
	pickBool(&out.History, overlay.History)
	pickBool(&out.GenerateLine, overlay.GenerateLine)
	if len(overlay.ExcludePackages) > 0 {
		out.ExcludePackages = overlay.ExcludePackages
	}
	if overlay.TemplateData != nil {
		if out.TemplateData == nil {
			out.TemplateData = &TemplateData{}
		}
		td := *out.TemplateData
		if overlay.TemplateData.BoilerplateFile != nil {
			td.BoilerplateFile = overlay.TemplateData.BoilerplateFile
		}
		out.TemplateData = &td
	}
	return &out
}

// LayersForFile — слои для одного интерфейса из файла конфигурации:
// defaults → for-package → for-interface (§4.1).
func LayersForFile(f *File, pkgPath, ifaceName string) []Layer {
	var layers []Layer
	if f.Defaults != nil {
		layers = append(layers, Layer{Name: "defaults", S: f.Defaults})
	}
	if pc, ok := f.Packages[pkgPath]; ok {
		if pc.Config != nil {
			layers = append(layers, Layer{Name: fmt.Sprintf(
				"for-package %s",
				pkgPath,
			), S: pc.Config})
		}
		if ic, ok := pc.Interfaces[ifaceName]; ok && ic.Config != nil {
			layers = append(layers, Layer{Name: fmt.Sprintf(
				"for-interface %s",
				ifaceName,
			), S: ic.Config})
		}
	}
	return layers
}
