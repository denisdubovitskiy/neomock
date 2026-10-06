package gen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/denisdubovitskiy/neomock/internal/config"
	"github.com/denisdubovitskiy/neomock/internal/loader"
)

// Explain — вывод команды explain: итоговые значения настроек интерфейса с
// источником каждого (§4.7).
func Explain(workDir, configPath, pkgPath, ifaceName string) (string, error) {
	mod, err := DetectModule(workDir)
	if err != nil {
		return "", err
	}
	file, _, err := loadConfigFile(workDir, Options{ConfigPath: configPath})
	if err != nil {
		return "", err
	}
	layers := []config.Layer{}
	if file != nil {
		layers = config.LayersForFile(file, pkgPath, ifaceName)
	}
	resolved := config.Resolve(layers...)

	source := resolved.Source
	iface, err := loader.Load(loader.Config{
		PackagePath:   pkgPath,
		InterfaceName: ifaceName,
		Dir:           workDir,
		InPackage:     resolved.InPackage,
		Source:        source,
		BuildTags:     splitTags(resolved.BuildTags),
	})
	if err != nil {
		return "", err
	}
	relDir, err := filepath.Rel(mod.Root, iface.Pkg.Dir)
	if err != nil {
		relDir = iface.Pkg.Dir
	}
	vars := config.TemplateVars{
		InterfaceName:        ifaceName,
		InterfaceSnake:       config.Snake(ifaceName),
		Mock:                 config.MockPrefix(ifaceName),
		SrcPackageName:       iface.Pkg.Name,
		SrcPackagePath:       iface.Pkg.Path,
		InterfaceDir:         iface.Pkg.Dir,
		InterfaceDirRelative: filepath.ToSlash(relDir),
		InterfaceFile:        iface.File,
	}
	structName, err := config.RenderTemplate(resolved.Structname, vars)
	if err != nil {
		return "", err
	}
	vars.StructName = structName
	render := func(tmpl string) string {
		out, err := config.RenderTemplate(tmpl, vars)
		if err != nil {
			return tmpl
		}
		return out
	}
	resolved.Dir = render(resolved.Dir)
	resolved.Filename = render(resolved.Filename)
	resolved.Structname = structName
	if resolved.InPackage {
		resolved.Pkgname = iface.Pkg.Name
	}
	return resolved.Explain(pkgPath + "." + ifaceName), nil
}

// SplitIfaceID разбирает аргумент explain <пакет.Интерфейс>: граница —
// последняя точка, после которой идёт сегмент с заглавной буквы.
func SplitIfaceID(id string) (pkgPath, ifaceName string, err error) {
	for i := strings.LastIndex(id, "."); i > 0; i = strings.LastIndex(id[:i], ".") {
		next := id[i+1:]
		if next != "" && strings.ToUpper(next[:1]) == next[:1] {
			return id[:i], next, nil
		}
	}
	return "", "", fmt.Errorf(
		"explain: ожидался формат <import/path>.<Interface>, получено %q",
		id,
	)
}
