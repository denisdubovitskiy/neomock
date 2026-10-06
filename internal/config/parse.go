package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// legacyKeys — ключи testify-наследия (mockery): удалены в ред. 2 (§4.2).
var legacyKeys = map[string]string{
	"unroll-variadic":    "variadic пробрасывается как есть, unroll не нужен",
	"with-expecter":      "EXPECT не генерируется: используется When/Then",
	"template":           "кастомные шаблоны вне v1: кастомизация — файлами (вне v1)",
	"packages-templates": "шаблоны пакетов удалены в ред. 2",
}

// Parse разбирает файл конфигурации: format — "yaml"|"toml".
// Оба формата дают одинаковую семантику: сначала generic-карта, затем одна
// и та же валидация схемы (§4.1).
func Parse(data []byte, format string) (*File, error) {
	var raw map[string]any
	var err error
	switch format {
	case "yaml":
		err = yaml.Unmarshal(data, &raw)
	case "toml":
		err = toml.Unmarshal(data, &raw)
	default:
		return nil, fmt.Errorf("config: неизвестный формат %q (yaml|toml)", format)
	}
	if err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", format, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("config: пустой файл конфигурации")
	}
	return parseFile(raw)
}

func parseFile(raw map[string]any) (*File, error) {
	if err := checkKeys(
		raw,
		"root",
		"version",
		"defaults",
		"packages",
	); err != nil {
		return nil, err
	}
	f := &File{}
	if v, ok := raw["version"]; ok {
		n, ok := asInt(v)
		if !ok || n != Version {
			return nil, fmt.Errorf(
				"config: version: ожидалось %d, получено %v (§4.2)",
				Version,
				v,
			)
		}
		f.Version = n
	} else {
		return nil, fmt.Errorf("config: version обязателен (version: %d)", Version)
	}
	if v, ok := raw["defaults"]; ok && v != nil {
		m, err := asMap(v, "defaults")
		if err != nil {
			return nil, err
		}
		f.Defaults, err = parseSettings(m, "defaults")
		if err != nil {
			return nil, err
		}
	}
	if v, ok := raw["packages"]; ok && v != nil {
		m, err := asMap(v, "packages")
		if err != nil {
			return nil, err
		}
		f.Packages = map[string]*PackageCfg{}
		for _, path := range sortedKeys(m) {
			sub, err := asMap(m[path], "packages."+path)
			if err != nil {
				return nil, err
			}
			pcfg, err := parsePackageCfg(sub, path)
			if err != nil {
				return nil, err
			}
			f.Packages[path] = pcfg
		}
	}
	return f, nil
}

func parsePackageCfg(m map[string]any, path string) (*PackageCfg, error) {
	if err := checkKeys(
		m,
		"packages."+path,
		"all",
		"include",
		"exclude",
		"interfaces",
		"config",
	); err != nil {
		return nil, err
	}
	p := &PackageCfg{}
	if v, ok := m["all"]; ok {
		b, ok := asBool(v)
		if !ok {
			return nil, fmt.Errorf("config: packages.%s.all: ожидается bool", path)
		}
		p.All = b
	}
	if v, ok := m["include"]; ok {
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("config: packages.%s.include: ожидается строка", path)
		}
		p.Include = s
	}
	if v, ok := m["exclude"]; ok {
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("config: packages.%s.exclude: ожидается строка", path)
		}
		p.Exclude = s
	}
	if v, ok := m["interfaces"]; ok {
		m2, err := asMap(v, "packages."+path+".interfaces")
		if err != nil {
			return nil, err
		}
		p.Interfaces = map[string]*IfaceCfg{}
		for _, name := range sortedKeys(m2) {
			sub, err := asMap(m2[name], fmt.Sprintf(
				"packages.%s.interfaces.%s",
				path,
				name,
			))
			if err != nil {
				return nil, err
			}
			icfg, err := parseIfaceCfg(sub, path, name)
			if err != nil {
				return nil, err
			}
			p.Interfaces[name] = icfg
		}
	}
	if v, ok := m["config"]; ok {
		sub, err := asMap(v, "packages."+path+".config")
		if err != nil {
			return nil, err
		}
		p.Config, err = parseSettings(sub, "packages."+path+".config")
		if err != nil {
			return nil, err
		}
	}
	if p.Exclude != "" && p.Include == "" {
		return nil, fmt.Errorf(
			"config: packages.%s: exclude без include — ошибка (§4.2)",
			path,
		)
	}
	// Ровно один селектор: all:true | interfaces | include (§4.2).
	n := 0
	if p.All {
		n++
	}
	if len(p.Interfaces) > 0 {
		n++
	}
	if p.Include != "" {
		n++
	}
	if n != 1 {
		return nil, fmt.Errorf(
			"config: packages.%s: требуется ровно один селектор — all:true, interfaces: или include: (найдено %d)",
			path,
			n,
		)
	}
	return p, nil
}

func parseIfaceCfg(m map[string]any, path, name string) (*IfaceCfg, error) {
	if err := checkKeys(m, fmt.Sprintf(
		"packages.%s.interfaces.%s",
		path,
		name,
	), "source-interface", "config"); err != nil {
		return nil, err
	}
	i := &IfaceCfg{}
	if v, ok := m["source-interface"]; ok {
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf(
				"config: packages.%s.interfaces.%s.source-interface: ожидается строка",
				path,
				name,
			)
		}
		i.SourceInterface = s
	}
	if v, ok := m["config"]; ok {
		sub, err := asMap(v, fmt.Sprintf(
			"packages.%s.interfaces.%s.config",
			path,
			name,
		))
		if err != nil {
			return nil, err
		}
		i.Config, err = parseSettings(sub, fmt.Sprintf(
			"packages.%s.interfaces.%s.config",
			path,
			name,
		))
		if err != nil {
			return nil, err
		}
	}
	return i, nil
}

// parseSettings разбирает блок настроек с валидацией ключей и значений (§4.2).
func parseSettings(m map[string]any, ctx string) (*Settings, error) {
	if err := checkLegacy(m, ctx); err != nil {
		return nil, err
	}
	if err := checkKeys(m, ctx,
		"dir", "filename", "pkgname", "structname", "source", "inpackage",
		"build-tags", "force-write", "verify-build", "strict-calls", "history",
		"generate-line", "recursive", "exclude-packages", "template-data",
	); err != nil {
		return nil, err
	}
	s := &Settings{}
	str := func(key string) (*string, error) {
		v, ok := m[key]
		if !ok {
			return nil, nil
		}
		sv, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("config: %s.%s: ожидается строка", ctx, key)
		}
		return &sv, nil
	}
	boolean := func(key string) (*bool, error) {
		v, ok := m[key]
		if !ok {
			return nil, nil
		}
		bv, ok := asBool(v)
		if !ok {
			return nil, fmt.Errorf("config: %s.%s: ожидается bool", ctx, key)
		}
		return &bv, nil
	}
	var err error
	if s.Dir, err = str("dir"); err != nil {
		return nil, err
	}
	if s.Filename, err = str("filename"); err != nil {
		return nil, err
	}
	if s.Pkgname, err = str("pkgname"); err != nil {
		return nil, err
	}
	if s.Structname, err = str("structname"); err != nil {
		return nil, err
	}
	if s.Source, err = str("source"); err != nil {
		return nil, err
	}
	if s.BuildTags, err = str("build-tags"); err != nil {
		return nil, err
	}
	if s.InPackage, err = boolean("inpackage"); err != nil {
		return nil, err
	}
	if s.ForceWrite, err = boolean("force-write"); err != nil {
		return nil, err
	}
	if s.VerifyBuild, err = boolean("verify-build"); err != nil {
		return nil, err
	}
	if s.StrictCalls, err = boolean("strict-calls"); err != nil {
		return nil, err
	}
	if s.History, err = boolean("history"); err != nil {
		return nil, err
	}
	if s.GenerateLine, err = boolean("generate-line"); err != nil {
		return nil, err
	}
	if s.Recursive, err = boolean("recursive"); err != nil {
		return nil, err
	}
	if v, ok := m["exclude-packages"]; ok {
		list, ok := asStringSlice(v)
		if !ok {
			return nil, fmt.Errorf(
				"config: %s.exclude-packages: ожидается список строк",
				ctx,
			)
		}
		s.ExcludePackages = list
	}
	if v, ok := m["template-data"]; ok {
		sub, err := asMap(v, ctx+".template-data")
		if err != nil {
			return nil, err
		}
		if err := checkKeys(
			sub,
			ctx+".template-data",
			"boilerplate-file",
		); err != nil {
			return nil, err
		}
		td := &TemplateData{}
		if bv, ok := sub["boilerplate-file"]; ok {
			sv, ok := asString(bv)
			if !ok {
				return nil, fmt.Errorf(
					"config: %s.template-data.boilerplate-file: ожидается строка",
					ctx,
				)
			}
			td.BoilerplateFile = &sv
		}
		s.TemplateData = td
	}
	if s.Source != nil {
		switch *s.Source {
		case "go", "test", "all":
		default:
			return nil, fmt.Errorf(
				"config: %s.source: допустимы go|test|all, получено %q",
				ctx,
				*s.Source,
			)
		}
	}
	return s, nil
}

// checkKeys сверяет ключи карты с известными; неизвестный ключ — ошибка с
// похожими вариантами (§4.2).
func checkKeys(m map[string]any, ctx string, known ...string) error {
	set := map[string]bool{}
	for _, k := range known {
		set[k] = true
	}
	for _, k := range sortedKeys(m) {
		if set[k] {
			continue
		}
		if hint, ok := legacyKeys[k]; ok {
			return fmt.Errorf(
				"config: %s: ключ %q удалён в ред. 2, см. миграцию (%s)",
				ctx,
				k,
				hint,
			)
		}
		return fmt.Errorf(
			"config: %s: неизвестный ключ %q%s",
			ctx,
			k,
			similarHint(k, known),
		)
	}
	return nil
}

func checkLegacy(m map[string]any, ctx string) error {
	for _, k := range sortedKeys(m) {
		if hint, ok := legacyKeys[k]; ok {
			return fmt.Errorf(
				"config: %s: ключ %q удалён в ред. 2, см. миграцию (%s)",
				ctx,
				k,
				hint,
			)
		}
	}
	return nil
}

// similarHint подсказывает похожие ключи (редакционное расстояние ≤ 2).
func similarHint(key string, known []string) string {
	var best []string
	for _, k := range known {
		if editDistance(key, k) <= 2 {
			best = append(best, k)
		}
	}
	if len(best) == 0 {
		return ""
	}
	sort.Strings(best)
	return fmt.Sprintf(", возможно, вы имели в виду: %s", strings.Join(best, ", "))
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

// --- приведение типов generic-карты ---

func asMap(v any, ctx string) (map[string]any, error) {
	if v == nil {
		// пустой блок (`config:` без значений) валиден
		return map[string]any{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config: %s: ожидается таблица", ctx)
	}
	return m, nil
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

func asStringSlice(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := asString(e)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
