package config

import (
	"fmt"
	"strings"
)

// GenFlags — флаги go:generate-строки (режим А, §4.3).
// Собственная командная строка: кавычки разбирает go generate, shell-семантики нет.
type GenFlags struct {
	// Interfaces — повторяемый --interface.
	Interfaces []string
	// Out — путь файла мока (обязателен в режиме А).
	Out string
	// Pkgname — имя пакета мока.
	Pkgname string
	// Structname — имя структуры мока.
	Structname string
	// Source — go|test|all.
	Source string
	// InPackage — мок в пакете источника.
	InPackage bool
	// StrictCalls — аналог strict-calls: true (§4.3).
	StrictCalls bool
	// History — аналог history: false (nil — не задано).
	History *bool
	// GenerateLine — аналог generate-line: вставлять ли //go:generate-шапку
	// (nil — не задано, false — не вставлять).
	GenerateLine *bool
	// BuildTags — теги сборки.
	BuildTags string
	// Boilerplate — файл шапки (лицензия).
	Boilerplate string
	// Config — явный файл конфигурации (режим Б).
	Config string
	// LogLevel — уровень логирования.
	LogLevel string
}

// ParseGenerateArgs разбирает аргументы go:generate-строки (режим А).
// Поддержка форм: --flag value, --flag=value, булевы --flag / --flag=true|false.
func ParseGenerateArgs(args []string) (*GenFlags, error) {
	f := &GenFlags{}
	takesValue := map[string]bool{
		"--interface": true, "--out": true, "--pkgname": true, "--structname": true,
		"--source": true, "--build-tags": true, "--boilerplate": true,
		"--config": true, "--log-level": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val := arg, ""
		hasVal := false
		if strings.HasPrefix(arg, "-") {
			if eq := strings.Index(arg, "="); eq >= 0 {
				name, val, hasVal = arg[:eq], arg[eq+1:], true
			}
		} else {
			return nil, fmt.Errorf(
				"go:generate: unexpected argument %q (expected --flag)",
				arg,
			)
		}
		// нормализация одиночного дефиса: -interface → --interface
		if strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") {
			name = "-" + name
		}
		if takesValue[name] {
			if !hasVal {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("go:generate: %s требует значение", name)
				}
				i++
				val = args[i]
			}
			switch name {
			case "--interface":
				f.Interfaces = append(f.Interfaces, val)
			case "--out":
				f.Out = val
			case "--pkgname":
				f.Pkgname = val
			case "--structname":
				f.Structname = val
			case "--source":
				f.Source = val
			case "--build-tags":
				f.BuildTags = val
			case "--boilerplate":
				f.Boilerplate = val
			case "--config":
				f.Config = val
			case "--log-level":
				f.LogLevel = val
			}
			continue
		}
		switch name {
		case "--inpackage":
			b, err := boolOpt(name, val, hasVal, true)
			if err != nil {
				return nil, err
			}
			f.InPackage = b
		case "--strict-calls":
			b, err := boolOpt(name, val, hasVal, true)
			if err != nil {
				return nil, err
			}
			f.StrictCalls = b
		case "--history":
			b, err := boolOpt(name, val, hasVal, true)
			if err != nil {
				return nil, err
			}
			f.History = &b
		case "--generate-line":
			b, err := boolOpt(name, val, hasVal, true)
			if err != nil {
				return nil, err
			}
			f.GenerateLine = &b
		default:
			return nil, fmt.Errorf("go:generate: unknown flag %q", name)
		}
	}
	if f.Source != "" && f.Source != "go" && f.Source != "test" && f.Source != "all" {
		return nil, fmt.Errorf(
			"go:generate: --source: допустимы go|test|all, получено %q",
			f.Source,
		)
	}
	return f, nil
}

func boolOpt(name, val string, hasVal, def bool) (bool, error) {
	if !hasVal {
		return def, nil
	}
	switch val {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, fmt.Errorf(
		"go:generate: %s: ожидается true|false, получено %q",
		name,
		val,
	)
}

// SettingsLayer превращает флаги режима А в слой слияния. По приоритету
// строка go:generate сильнее файла (§4.1).
func (f *GenFlags) SettingsLayer() *Settings {
	s := &Settings{}
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	s.Pkgname = str(f.Pkgname)
	s.Structname = str(f.Structname)
	s.Source = str(f.Source)
	s.BuildTags = str(f.BuildTags)
	if f.InPackage {
		b := true
		s.InPackage = &b
	}
	if f.StrictCalls {
		b := true
		s.StrictCalls = &b
	}
	s.History = f.History
	s.GenerateLine = f.GenerateLine
	if f.Boilerplate != "" {
		s.TemplateData = &TemplateData{BoilerplateFile: str(f.Boilerplate)}
	}
	return s
}
