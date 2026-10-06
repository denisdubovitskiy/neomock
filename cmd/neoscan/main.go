// Command neoscan — одноразовый инструмент валидации: находит .mockery.yml /
// .mockery.yaml в заданных корнях и проверяет, что для каждого настроенного
// в них интерфейса neomock может сгенерировать мок (loader + emitter +
// format.Source). Результаты агрегируются по проектам и классам ошибок.
//
// Использование: go run ./cmd/neoscan ~/projects
package main

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"

	"github.com/denisdubovitskiy/neomock/internal/emitter"
	"github.com/denisdubovitskiy/neomock/internal/format"
	"github.com/denisdubovitskiy/neomock/internal/loader"
)

// ---- разбор .mockery.yml (нас интересует только состав целей) ----

type pkgEntry struct {
	Config struct {
		All       *bool `yaml:"all"`
		Recursive *bool `yaml:"recursive"`
	} `yaml:"config"`
	All        *bool                `yaml:"all"`
	Recursive  *bool                `yaml:"recursive"`
	Interfaces map[string]yaml.Node `yaml:"interfaces"`
}

type mockeryCfg struct {
	All       *bool               `yaml:"all"`
	Recursive *bool               `yaml:"recursive"`
	Packages  map[string]pkgEntry `yaml:"packages"`
}

// task — одна проверка генерации.
type task struct {
	Dir     string // каталог модуля (контекст packages.Load)
	Project string // имя проекта для отчёта
	PkgPath string
	Iface   string
}

type result struct {
	task
	OK  bool
	Err error
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{os.Getenv("HOME") + "/projects"}
	}

	// 1. Найти конфиги.
	var cfgFiles []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(
			path string,
			d os.DirEntry,
			err error,
		) error {
			if err != nil {
				return nil // недоступные поддеревья пропускаем
			}
			if d.IsDir() {
				switch d.Name() {
				case "vendor", "node_modules", ".git", "testdata", "sandbox":
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() == ".mockery.yml" || d.Name() == ".mockery.yaml" {
				cfgFiles = append(cfgFiles, path)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "walk %s: %v\n", root, err)
		}
	}
	fmt.Fprintf(os.Stderr, "конфигов найдено: %d\n", len(cfgFiles))

	// 2. Собрать задачи.
	var tasks []task
	for _, cfgPath := range cfgFiles {
		ts, err := parseConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", cfgPath, err)
			continue
		}
		tasks = append(tasks, ts...)
	}
	fmt.Fprintf(os.Stderr, "интерфейсов к проверке: %d\n", len(tasks))

	// 3. Прогнать параллельно.
	var (
		mu      sync.Mutex
		results []result
		done    atomic.Int64
		sem     = make(chan struct{}, 8)
		wg      sync.WaitGroup
	)
	for _, t := range tasks {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok, err := false, error(nil)
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
					}
				}()
				ok, err = check(t)
			}()
			mu.Lock()
			results = append(results, result{task: t, OK: ok, Err: err})
			mu.Unlock()
			if n := done.Add(1); n%25 == 0 {
				fmt.Fprintf(os.Stderr, "  ...проверено %d/%d\n", n, len(tasks))
			}
		}()
	}
	wg.Wait()

	// 4. Отчёт.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Project != results[j].Project {
			return results[i].Project < results[j].Project
		}
		if results[i].OK != results[j].OK {
			return !results[i].OK
		}
		return results[i].Iface < results[j].Iface
	})

	byProject := map[string][2]int{}
	for _, r := range results {
		p := byProject[r.Project]
		if r.OK {
			p[0]++
		} else {
			p[1]++
		}
		byProject[r.Project] = p
	}

	totalOK, totalFail := 0, 0
	fmt.Println("\n=== по проектам ===")
	projects := make([]string, 0, len(byProject))
	for p := range byProject {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	for _, p := range projects {
		c := byProject[p]
		totalOK += c[0]
		totalFail += c[1]
		fmt.Printf("%-45s ok: %3d  fail: %3d\n", p, c[0], c[1])
	}
	fmt.Printf("%-45s ok: %3d  fail: %3d\n", "ИТОГО", totalOK, totalFail)

	fmt.Println("\n=== падения ===")
	for _, r := range results {
		if r.OK {
			continue
		}
		msg := ""
		if r.Err != nil {
			msg = r.Err.Error()
			if len(msg) > 240 {
				msg = msg[:240] + "..."
			}
			msg = strings.ReplaceAll(msg, "\n", " | ")
		}
		fmt.Printf("FAIL %s %s.%s\n     %s\n", r.Project, r.PkgPath, r.Iface, msg)
	}

	// 5. Классы ошибок — сгруппированные причины.
	fmt.Println("\n=== классы ошибок ===")
	classes := map[string]int{}
	for _, r := range results {
		if r.OK {
			continue
		}
		classes[errClass(r.Err)]++
	}
	var cls []string
	for c := range classes {
		cls = append(cls, c)
	}
	sort.Slice(
		cls,
		func(i, j int) bool { return classes[cls[i]] > classes[cls[j]] },
	)
	for _, c := range cls {
		fmt.Printf("%4d  %s\n", classes[c], c)
	}
}

// boolVal разрешает значение с наследованием корневого дефолта.
func boolVal(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// parseConfig разбирает один .mockery.yml и возвращает задачи проверки.
func parseConfig(cfgPath string) ([]task, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg mockeryCfg
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	dir := filepath.Dir(cfgPath)
	project := filepath.Base(dir)

	var tasks []task
	pkgPaths := make([]string, 0, len(cfg.Packages))
	for p := range cfg.Packages {
		pkgPaths = append(pkgPaths, p)
	}
	sort.Strings(pkgPaths)

	for _, pkgPath := range pkgPaths {
		entry := cfg.Packages[pkgPath]
		all := boolVal(entry.All, boolVal(entry.Config.All, false))
		recursive := boolVal(entry.Recursive, boolVal(entry.Config.Recursive, false))

		if len(entry.Interfaces) > 0 {
			names := make([]string, 0, len(entry.Interfaces))
			for name := range entry.Interfaces {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				tasks = append(
					tasks,
					task{Dir: dir, Project: project, PkgPath: pkgPath, Iface: name},
				)
			}
			continue
		}

		// all: true (recursive — обходим поддерево пакетов целиком)
		if all || recursive {
			pattern := pkgPath
			if recursive && !strings.HasSuffix(pattern, "/...") {
				pattern += "/..."
			}
			ifaces, err := enumerate(dir, pattern)
			if err != nil {
				fmt.Fprintf(os.Stderr, "enumerate %s (%s): %v\n", pkgPath, project, err)
				continue
			}
			for _, ifc := range ifaces {
				tasks = append(
					tasks,
					task{Dir: dir, Project: project, PkgPath: ifc.pkgPath, Iface: ifc.name},
				)
			}
		}
	}
	return tasks, nil
}

// check выполняет одну проверку: loader → emitter → format.Source.
func check(t task) (bool, error) {
	inPackage := false
	iface, err := loader.Load(
		loader.Config{PackagePath: t.PkgPath, InterfaceName: t.Iface, Dir: t.Dir},
	)
	if err != nil && strings.Contains(err.Error(), "неэкспортирован") {
		// unexported-интерфейс — валидный кейс с inpackage (§3.3)
		inPackage = true
		iface, err = loader.Load(
			loader.Config{
				PackagePath:   t.PkgPath,
				InterfaceName: t.Iface,
				Dir:           t.Dir,
				InPackage:     true,
			},
		)
	}
	if err != nil {
		return false, err
	}

	opts := emitter.Options{Package: "mocks"}
	if inPackage {
		opts = emitter.Options{Package: iface.Pkg.Name, SelfPath: iface.Pkg.Path}
	}
	code, err := emitter.Render(iface, opts)
	if err != nil {
		return false, err
	}
	if _, err := format.Source([]byte(code)); err != nil {
		return false, err
	}
	return true, nil
}

// ifaceRef — интерфейс, найденный перечислением по пакету.
type ifaceRef struct {
	pkgPath string
	name    string
}

// enumerate возвращает все интерфейсы пакетов, соответствующих pattern.
func enumerate(dir, pattern string) ([]ifaceRef, error) {
	pkgs, err := packages.Load(&packages.Config{
		Dir:  dir,
		Mode: packages.NeedName | packages.NeedTypes,
	}, pattern)
	if err != nil {
		return nil, err
	}
	var out []ifaceRef
	seen := map[string]bool{}
	for _, p := range pkgs {
		// тест-варианты и внешние тест-пакеты не мокаем per se
		if strings.Contains(p.PkgPath, " [") || strings.HasSuffix(p.PkgPath, "_test") {
			continue
		}
		if p.Types == nil || p.Types.Scope() == nil {
			continue
		}
		for _, name := range p.Types.Scope().Names() {
			obj := p.Types.Scope().Lookup(name)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			if _, isIface := tn.Type().Underlying().(*types.Interface); !isIface {
				continue
			}
			key := p.PkgPath + "." + name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ifaceRef{pkgPath: p.PkgPath, name: name})
		}
	}
	return out, nil
}

// errClass сворачивает ошибку в класс для агрегации.
func errClass(err error) string {
	if err == nil {
		return "?"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "не найден в пакете"):
		return "тип не найден (в т.ч. только в *_test.go)"
	case strings.Contains(msg, "неэкспортирован"):
		return "unexported-интерфейс (нужен inpackage)"
	case strings.Contains(msg, "ошибки пакета"):
		return "пакет не компилируется"
	case strings.Contains(msg, "unsupported type"):
		return "тип не поддержан рендером"
	case strings.Contains(msg, "conflicts with reserved"):
		return "зарезервированное имя (§3.2.6)"
	case strings.Contains(msg, "history: false"):
		return "коллизия имени истории (§3.2.6)"
	case strings.Contains(msg, "conflict method"):
		return "конфликт встраивания"
	case strings.Contains(msg, "format:"):
		return "результат не форматируется (синтаксис генерата)"
	case strings.Contains(msg, "not an interface"):
		return "тип не интерфейс"
	case strings.Contains(msg, "packages.Load"):
		return "ошибка загрузки пакета"
	default:
		if len(msg) > 80 {
			msg = msg[:80]
		}
		return "прочее: " + msg
	}
}
