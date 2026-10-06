package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

// TemplateVars — переменные шаблонов dir/filename/pkgname/structname (§4.2).
type TemplateVars struct {
	// InterfaceName — имя интерфейса (Store).
	InterfaceName string
	// InterfaceSnake — snake_case имени (response_writer).
	InterfaceSnake string
	// Mock — "Mock" для экспортированного, "mock" для приватного интерфейса.
	Mock string
	// StructName — уже отрендеренное имя структуры (доступно в dir/filename).
	StructName string
	// SrcPackageName — имя пакета источника.
	SrcPackageName string
	// SrcPackagePath — import path пакета источника.
	SrcPackagePath string
	// InterfaceDir — каталог исходников интерфейса (абсолютный путь).
	InterfaceDir string
	// InterfaceDirRelative — каталог интерфейса относительно корня модуля.
	InterfaceDirRelative string
	// InterfaceFile — файл объявления интерфейса.
	InterfaceFile string
}

// TemplateFuncs — функции шаблонов: lower/upper/title/snake/base/dir/replaceAll (§4.2).
var TemplateFuncs = template.FuncMap{
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	"title": func(s string) string {
		r := []rune(s)
		if len(r) == 0 {
			return s
		}
		return string(unicode.ToUpper(r[0])) + string(r[1:])
	},
	"snake": Snake,
	"base":  filepath.Base,
	"dir":   filepath.Dir,
	"replaceAll": func(old, new, s string) string {
		return strings.ReplaceAll(s, old, new)
	},
}

// RenderTemplate выполняет шаблон один проход, без вложенности (§4.2).
func RenderTemplate(tmpl string, vars TemplateVars) (string, error) {
	t, err := template.New(
		"neomock",
	).Funcs(TemplateFuncs).Option("missingkey=error").Parse(
		tmpl,
	)
	if err != nil {
		return "", fmt.Errorf("config: шаблон %q: %w", tmpl, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", fmt.Errorf("config: шаблон %q: %w", tmpl, err)
	}
	return b.String(), nil
}

// Snake переводит CamelCase в snake_case: ResponseWriter → response_writer,
// HTTPServer → http_server, IDKey → id_key.
func Snake(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if !unicode.IsUpper(prev) || nextLower {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// MockPrefix — "Mock" для экспортированного имени, "mock" для приватного (§3.3).
func MockPrefix(ifaceName string) string {
	if ifaceName == "" {
		return "Mock"
	}
	r := []rune(ifaceName)[0]
	if unicode.IsUpper(r) {
		return "Mock"
	}
	return "mock"
}
