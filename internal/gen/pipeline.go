package gen

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/denisdubovitskiy/neomock/internal/config"
	"github.com/denisdubovitskiy/neomock/internal/emitter"
	"github.com/denisdubovitskiy/neomock/internal/format"
	"github.com/denisdubovitskiy/neomock/internal/loader"
	"github.com/denisdubovitskiy/neomock/internal/model"
)

// Mode — режим конвейера: запись на диск или сравнение в памяти (§5).
type Mode int

const (
	// ModeGenerate — генерация с записью (skip-write-if-equal).
	ModeGenerate Mode = iota
	// ModeCheck — генерация в память, diff, exit 1 при расхождении.
	ModeCheck
)

// Options — вход конвейера.
type Options struct {
	// ConfigPath — явный --config / NEOMOCK_CONFIG (§4.6).
	ConfigPath string
	// Patterns — позиционные аргументы [пакеты...]: фильтр секций конфига.
	Patterns []string
	// Clean — удалить orphaned-моки (только полный прогон с конфигом, С11).
	Clean bool
	// GoGenA — режим А go:generate (флаги строки); nil — не он.
	GoGenA *config.GenFlags
	// GoGenB — режим Б go:generate (короткая строка, секция пакета).
	GoGenB bool
}

// FileResult — итог по одному файлу мока.
type FileResult struct {
	// PkgPath — import path пакета-источника интерфейса.
	PkgPath string
	// Iface — имя интерфейса, замоканированного в файл.
	Iface string
	// Path — путь к файлу мока относительно рабочей директории.
	Path string
	// Status — итог записи: created|updated|unchanged|failed.
	Status format.Status
	// Diff — unified diff для check при расхождении.
	Diff string
	// Missing — файл отсутствует (check).
	Missing bool
	// Err — причина ошибки генерации, если Status == failed.
	Err error
}

// Result — итог прогона.
type Result struct {
	// Files — итоги по всем сгенерированным/проверенным файлам.
	Files []FileResult
	// Warnings — предупреждения прогона (исключения, обходы ограничений).
	Warnings []string
	// Orphaned — файлы с маркером neomock, не соответствующие ни одному
	// сконфигурированному интерфейсу (С11).
	Orphaned []string
	// Deleted — удалены флагом --clean.
	Deleted []string
	// Notice — причина пропуска orphaned-скана (частичный прогон / нет конфига).
	Notice string
	// Stale — число расхождений для check.
	Stale int
}

// ifaceTask — один интерфейс к генерации.
type ifaceTask struct {
	name            string
	sourceInterface string
	layer           config.Layer
}

// pkgEntry — пакет к обработке.
type pkgEntry struct {
	pkgPath string
	cfg     *config.PackageCfg // nil в режиме А
	tasks   []ifaceTask        // явный список (режим А / interfaces:); nil — селектор
	gogen   *config.GenFlags
}

type runner struct {
	mode    Mode
	workDir string
	mod     *ModuleInfo
	opts    Options
	file    *config.File // nil — бесконфиг-режим (go:generate)
	mu      sync.Mutex
	tasks   []*mockTask // все сгенерированные задачи (для verify-build и orphaned-скана)
	res     *Result
}

// Run выполняет конвейер generate/check.
func Run(mode Mode, workDir string, opts Options) (*Result, error) {
	mod, err := DetectModule(workDir)
	if err != nil {
		return nil, err
	}
	file, _, err := loadConfigFile(workDir, opts)
	if err != nil {
		return nil, err
	}
	r := &runner{
		mode:    mode,
		workDir: workDir,
		mod:     mod,
		opts:    opts,
		file:    file,
		res:     &Result{},
	}

	entries, err := r.buildEntries()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf(
			"gen: нет пакетов для генерации (проверьте конфиг и фильтр пакетов)",
		)
	}

	// Параллелизм по пакетам (§5).
	limit := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for _, e := range entries {
		wg.Add(1)
		go func(e pkgEntry) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			if err := r.processEntry(e); err != nil {
				errOnce.Do(func() { firstErr = err })
			}
		}(e)
	}
	wg.Wait()
	if firstErr != nil {
		return r.res, firstErr
	}

	sort.Slice(
		r.res.Files,
		func(a, b int) bool { return r.res.Files[a].Path < r.res.Files[b].Path },
	)

	// verify-build: go build затронутых пакетов после записи (§4.2).
	if r.mode == ModeGenerate {
		if err := r.verifyBuild(); err != nil {
			return r.res, err
		}
	}

	// orphaned-скан: только полный прогон с файлом конфигурации (С11).
	if err := r.scanOrphans(entries); err != nil {
		return r.res, err
	}
	return r.res, nil
}

// loadConfigFile — файл конфигурации или nil (бесконфиг-режим go:generate).
func loadConfigFile(workDir string, opts Options) (*config.File, string, error) {
	explicit := opts.ConfigPath
	if explicit == "" {
		explicit = os.Getenv(config.EnvConfig)
	}
	path, format, err := config.Discover(explicit, workDir)
	if err != nil {
		return nil, "", err
	}
	if path == "" {
		return nil, "", nil
	}
	f, err := config.LoadFile(path, format)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// buildEntries — список пакетов к обработке.
func (r *runner) buildEntries() ([]pkgEntry, error) {
	switch {
	case r.opts.GoGenA != nil:
		return r.entriesModeA()
	case r.opts.GoGenB:
		return r.entriesModeB()
	default:
		return r.entriesFileMode()
	}
}

// entriesModeA — режим А: пакет = cwd, интерфейсы из флагов (§4.3).
func (r *runner) entriesModeA() ([]pkgEntry, error) {
	f := r.opts.GoGenA
	if len(f.Interfaces) == 0 {
		return nil, fmt.Errorf("gen: go:generate режим А: требуется --interface")
	}
	if f.Out == "" {
		return nil, fmt.Errorf("gen: go:generate режим А: требуется --out")
	}
	pkgPath, err := ImportPathOf(r.workDir)
	if err != nil {
		return nil, err
	}
	if len(f.Interfaces) > 1 && !strings.HasSuffix(f.Out, "/") {
		return nil, fmt.Errorf(
			"gen: несколько --interface требуют --out с каталогом (оканчивается на /)",
		)
	}
	var tasks []ifaceTask
	for _, name := range f.Interfaces {
		tasks = append(tasks, ifaceTask{name: name})
	}
	return []pkgEntry{{pkgPath: pkgPath, tasks: tasks, gogen: f}}, nil
}

// entriesModeB — режим Б: короткая строка, секция пакета из конфига (§4.3).
func (r *runner) entriesModeB() ([]pkgEntry, error) {
	if r.file == nil {
		return nil, fmt.Errorf(
			"gen: go:generate режим Б требует файл конфигурации (.neomock.yml/.toml) или --interface",
		)
	}
	pkgPath, err := ImportPathOf(r.workDir)
	if err != nil {
		return nil, err
	}
	pcfg, ok := r.file.Packages[pkgPath]
	if !ok {
		return nil, fmt.Errorf(
			"gen: go:generate режим Б: в конфиге нет секции пакета %q — добавьте её или укажите --interface",
			pkgPath,
		)
	}
	return []pkgEntry{{pkgPath: pkgPath, cfg: pcfg}}, nil
}

// entriesFileMode — секции конфига с фильтром позиционных паттернов.
func (r *runner) entriesFileMode() ([]pkgEntry, error) {
	if r.file == nil {
		return nil, fmt.Errorf(
			"gen: файл конфигурации не найден (.neomock.yml/.yaml/.toml) и нет go:generate-аргументов (§4.1)",
		)
	}
	filter, err := r.patternFilter()
	if err != nil {
		return nil, err
	}
	var entries []pkgEntry
	for _, path := range sortedPkgKeys(r.file.Packages) {
		pcfg := r.file.Packages[path]
		if strings.HasSuffix(path, "/...") {
			expanded, err := r.expandRecursive(path, pcfg)
			if err != nil {
				return nil, err
			}
			for _, p := range expanded {
				if filter != nil && !filter[p] {
					continue
				}
				entries = append(entries, pkgEntry{pkgPath: p, cfg: pcfg})
			}
			continue
		}
		if filter != nil && !filter[path] {
			continue
		}
		entries = append(entries, pkgEntry{pkgPath: path, cfg: pcfg})
	}
	return entries, nil
}

// patternFilter — множество import path из позиционных аргументов.
func (r *runner) patternFilter() (map[string]bool, error) {
	if len(r.opts.Patterns) == 0 {
		return nil, nil
	}
	set := map[string]bool{}
	for _, p := range r.opts.Patterns {
		paths, err := ExpandPattern(r.workDir, p)
		if err != nil {
			return nil, err
		}
		for _, ip := range paths {
			set[ip] = true
		}
	}
	return set, nil
}

// expandRecursive — раскрытие паттерна <pkg>/... : только свой модуль +
// recursive:true, исключения exclude-packages (§3.3).
func (r *runner) expandRecursive(path string, pcfg *config.PackageCfg) ([]string, error) {
	if pcfg.Config == nil || pcfg.Config.Recursive == nil || !*pcfg.Config.Recursive {
		return nil, fmt.Errorf(
			"gen: паттерн %q требует recursive:true в config пакета (§3.3)",
			path,
		)
	}
	paths, err := ExpandPattern(r.workDir, path)
	if err != nil {
		return nil, err
	}
	var excludes []*regexp.Regexp
	if pcfg.Config != nil {
		for _, ex := range pcfg.Config.ExcludePackages {
			re, err := regexp.Compile(ex)
			if err != nil {
				return nil, fmt.Errorf("gen: exclude-packages %q: %w", ex, err)
			}
			excludes = append(excludes, re)
		}
	}
	var out []string
	for _, p := range paths {
		if !strings.HasPrefix(p, r.mod.Path+"/") && p != r.mod.Path {
			return nil, fmt.Errorf(
				"gen: рекурсивный обход %q за пределами текущего модуля запрещён (%q) — перечислите внешние пакеты явно (§3.3)",
				path,
				p,
			)
		}
		skip := false
		for _, re := range excludes {
			if re.MatchString(p) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// processEntry — загрузка пакета, селекция интерфейсов, генерация.
func (r *runner) processEntry(e pkgEntry) error {
	source := r.sourceFor(e)
	info, err := loader.LoadPackage(loader.Config{
		PackagePath: e.pkgPath,
		Dir:         r.workDir,
		Source:      source,
	})
	if err != nil {
		return fmt.Errorf("gen: пакет %s: %w", e.pkgPath, err)
	}
	r.addWarnings(info.Warnings...)

	tasks, err := r.selectInterfaces(e, info)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if err := r.processIface(e, t, info); err != nil {
			return fmt.Errorf("gen: %s.%s: %w", e.pkgPath, t.name, err)
		}
	}
	return nil
}

// sourceFor — режим source для загрузки пакета: максимальный охват из
// defaults, config пакета и config интерфейсов (иначе интерфейс из
// *_test.go с source:test не попадёт в селекцию, §3.3/С4).
func (r *runner) sourceFor(e pkgEntry) string {
	src := "go"
	widen := func(s string) {
		switch s {
		case "all":
			src = "all"
		case "test":
			if src == "go" {
				src = "test"
			}
		}
	}
	if r.file != nil && r.file.Defaults != nil && r.file.Defaults.Source != nil {
		widen(*r.file.Defaults.Source)
	}
	if e.cfg != nil {
		if e.cfg.Config != nil && e.cfg.Config.Source != nil {
			widen(*e.cfg.Config.Source)
		}
		// Селектор interfaces + source:test|all у любого интерфейса:
		// перечисление — union (all), а фактический source применяется на
		// загрузку каждого интерфейса (иначе интерфейс из *_test.go с
		// source:test не попадёт в список, §3.3/С4).
		for _, ic := range e.cfg.Interfaces {
			if ic.Config != nil && ic.Config.Source != nil && *ic.Config.Source != "go" {
				src = "all"
				break
			}
		}
	}
	if e.gogen != nil && e.gogen.Source != "" {
		widen(e.gogen.Source)
	}
	return src
}

// selectInterfaces — селектор all/include/exclude/interfaces + директива +
// constraint-пропуск (§3.3, §4.2, §4.5).
func (r *runner) selectInterfaces(e pkgEntry, info *loader.PackageInfo) ([]ifaceTask, error) {
	byName := map[string]loader.InterfaceInfo{}
	for _, i := range info.Interfaces {
		byName[i.Name] = i
	}

	// Явный список: режим А или interfaces: в конфиге.
	if len(e.tasks) > 0 {
		var out []ifaceTask
		for _, t := range e.tasks {
			ii, ok := byName[t.name]
			if !ok {
				return nil, fmt.Errorf(
					"gen: интерфейс %q не найден в пакете %s (source:%s; проверьте source:go|test|all, §3.3/С4)",
					t.name,
					e.pkgPath,
					r.sourceFor(e),
				)
			}
			if ii.IsConstraint {
				return nil, fmt.Errorf(
					"gen: интерфейс %q — constraint, мокирование невозможно (§3.3)",
					t.name,
				)
			}
			if ii.Disabled {
				r.addWarnings(fmt.Sprintf(
					"gen: интерфейс %s.%s исключён директивой //neomock:generate=false (§4.5)",
					e.pkgPath,
					t.name,
				))
				continue
			}
			out = append(out, t)
		}
		return out, nil
	}

	// interfaces: из конфига пакета. Имя-ключ может быть алиасом с
	// source-interface — тогда проверяется существование источника (С6).
	if e.cfg != nil && len(e.cfg.Interfaces) > 0 {
		var out []ifaceTask
		for _, name := range sortedIfaceKeys(e.cfg.Interfaces) {
			icfg := e.cfg.Interfaces[name]
			lookup := name
			if icfg.SourceInterface != "" {
				lookup = icfg.SourceInterface
			}
			ii, ok := byName[lookup]
			if !ok {
				return nil, fmt.Errorf(
					"gen: интерфейс %q из конфига не найден в пакете %s (source:%s, §3.3/С4)",
					lookup,
					e.pkgPath,
					r.sourceFor(e),
				)
			}
			if ii.IsConstraint {
				return nil, fmt.Errorf(
					"gen: интерфейс %q — constraint, мокирование невозможно (§3.3)",
					lookup,
				)
			}
			if ii.Disabled {
				r.addWarnings(fmt.Sprintf(
					"gen: интерфейс %s.%s исключён директивой //neomock:generate=false (§4.5)",
					e.pkgPath,
					lookup,
				))
				continue
			}
			out = append(out, ifaceTask{name: name, sourceInterface: icfg.SourceInterface, layer: config.Layer{
				Name: fmt.Sprintf("for-interface %s", name), S: icfg.Config,
			}})
		}
		return out, nil
	}

	// all / include+exclude.
	var include, exclude *regexp.Regexp
	if e.cfg != nil {
		if e.cfg.Include != "" {
			re, err := regexp.Compile(e.cfg.Include)
			if err != nil {
				return nil, fmt.Errorf("gen: include %q: %w", e.cfg.Include, err)
			}
			include = re
		}
		if e.cfg.Exclude != "" {
			re, err := regexp.Compile(e.cfg.Exclude)
			if err != nil {
				return nil, fmt.Errorf("gen: exclude %q: %w", e.cfg.Exclude, err)
			}
			exclude = re
		}
	}
	var out []ifaceTask
	for _, ii := range info.Interfaces {
		if ii.IsConstraint {
			r.addWarnings(fmt.Sprintf(
				"gen: интерфейс %s.%s пропущен — constraint-интерфейсы не мокаются (§3.3)",
				e.pkgPath,
				ii.Name,
			))
			continue
		}
		if ii.Disabled {
			continue
		}
		if include != nil && !include.MatchString(ii.Name) {
			continue
		}
		if exclude != nil && exclude.MatchString(ii.Name) {
			continue
		}
		out = append(out, ifaceTask{name: ii.Name})
	}
	return out, nil
}

// processIface — резолв настроек, рендер, загрузка интерфейса, запись/сравнение.
func (r *runner) processIface(
	e pkgEntry,
	t ifaceTask,
	info *loader.PackageInfo,
) error {
	var layers []config.Layer
	if r.file != nil {
		layers = config.LayersForFile(r.file, e.pkgPath, t.name)
	}
	if e.gogen != nil {
		layers = append(
			layers,
			config.Layer{Name: "go:generate", S: e.gogen.SettingsLayer()},
		)
	}
	resolved := config.Resolve(layers...)

	iface, err := loader.Load(loader.Config{
		PackagePath:     e.pkgPath,
		InterfaceName:   t.name,
		SourceInterface: t.sourceInterface,
		Dir:             r.workDir,
		InPackage:       resolved.InPackage,
		Source:          resolved.Source,
		BuildTags:       splitTags(resolved.BuildTags),
	})
	if err != nil {
		return err
	}

	out, err := r.renderOne(e, t, resolved, iface)
	if err != nil {
		return err
	}
	out.verify = resolved.VerifyBuild
	r.mu.Lock()
	r.tasks = append(r.tasks, out)
	r.mu.Unlock()

	switch r.mode {
	case ModeCheck:
		return r.checkFile(out)
	default:
		return r.writeFile(out)
	}
}

// mockTask — подготовленный к записи/сравнению файл.
type mockTask struct {
	pkgPath string
	iface   string
	path    string // абсолютный путь файла мока
	pkgDir  string // каталог мока (для orphaned-скана)
	selfPkg string // import path пакета мока (для verify-build)
	content []byte
	force   bool
	verify  bool // verify-build для этого пакета
}

// renderOne — шаблоны, placement-валидация, эмиссия.
func (r *runner) renderOne(
	e pkgEntry,
	t ifaceTask,
	resolved config.Resolved,
	iface *model.Interface,
) (*mockTask, error) {
	ifaceDir := iface.Pkg.Dir
	if ifaceDir == "" {
		return nil, fmt.Errorf("не определён каталог исходников интерфейса")
	}
	relDir, err := filepath.Rel(r.mod.Root, ifaceDir)
	if err != nil {
		relDir = ifaceDir
	}
	vars := config.TemplateVars{
		InterfaceName:        t.name,
		InterfaceSnake:       config.Snake(t.name),
		Mock:                 config.MockPrefix(t.name),
		SrcPackageName:       iface.Pkg.Name,
		SrcPackagePath:       iface.Pkg.Path,
		InterfaceDir:         ifaceDir,
		InterfaceDirRelative: filepath.ToSlash(relDir),
		InterfaceFile:        iface.File,
	}

	structName, err := config.RenderTemplate(resolved.Structname, vars)
	if err != nil {
		return nil, err
	}
	vars.StructName = structName
	dir, err := config.RenderTemplate(resolved.Dir, vars)
	if err != nil {
		return nil, err
	}
	filename, err := config.RenderTemplate(resolved.Filename, vars)
	if err != nil {
		return nil, err
	}
	pkgname := resolved.Pkgname

	// Режим А: --out задаёт файл напрямую и перекрывает dir/filename конфига —
	// строка инициировала запуск и сильнее файла (§4.1, §4.3).
	if e.gogen != nil {
		outAbs := e.gogen.Out
		if !filepath.IsAbs(outAbs) {
			outAbs = filepath.Join(r.workDir, outAbs)
		}
		dir = filepath.Dir(filepath.Clean(outAbs))
		filename = filepath.Base(filepath.Clean(outAbs))
	}

	// Placement rules (§3.3): внешний пакет — dir внутрь модуля, inpackage запрещён.
	if !iface.Pkg.InModule {
		if resolved.InPackage {
			return nil, fmt.Errorf(
				"пакет %s вне текущего модуля — inpackage физически невозможен (§3.3)",
				e.pkgPath,
			)
		}
		if e.gogen == nil && resolved.Dir == config.BuiltinDir() {
			return nil, fmt.Errorf(
				"пакет %s вне текущего модуля, укажите dir внутри проекта, например ./internal/testmocks (§3.3/С5)",
				e.pkgPath,
			)
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.mod.Root, dir)
	}
	dir = filepath.Clean(dir)
	if !isUnder(dir, r.mod.Root) {
		return nil, fmt.Errorf(
			"каталог мока %s вне текущего модуля (%s) — укажите dir внутри проекта (§3.3)",
			dir,
			r.mod.Root,
		)
	}

	// inpackage: pkgname равен имени исходника или пуст (§4.2).
	if resolved.InPackage {
		if pkgname != "" && pkgname != iface.Pkg.Name {
			return nil, fmt.Errorf(
				"inpackage:true требует pkgname %q (имя исходника) или пустой, получено %q (§4.2)",
				iface.Pkg.Name,
				pkgname,
			)
		}
		pkgname = iface.Pkg.Name
	} else if pkgname == "" {
		return nil, fmt.Errorf("pkgname обязателен при inpackage:false (§4.2)")
	}

	// source:test|all с filename без _test.go — предупреждение (§4.2).
	if (resolved.Source == "test" || resolved.Source == "all") && !strings.HasSuffix(filename, "_test.go") {
		r.addWarnings(fmt.Sprintf(
			"gen: %s.%s: source:%s, но filename %s не оканчивается на _test.go — мок попадёт в прод-сборку (§4.2)",
			e.pkgPath,
			t.name,
			resolved.Source,
			filename,
		))
	}

	selfPath := e.pkgPath
	if !resolved.InPackage {
		rel, err := filepath.Rel(r.mod.Root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("каталог мока %s вне модуля", dir)
		}
		selfPath = r.mod.Path + "/" + filepath.ToSlash(rel)
	}

	opts := emitter.Options{
		Package:        pkgname,
		SelfPath:       selfPath,
		StructName:     structName,
		StrictCalls:    resolved.StrictCalls,
		DisableHistory: !resolved.History,
	}
	if resolved.GenerateLine {
		if e.gogen != nil {
			// режим А: шапка с точными флагами (эмиттер соберёт по OutPath)
			opts.OutPath = e.gogen.Out
			opts.StructName = ""
			if structName != "Mock"+iface.Name {
				opts.StructName = structName
			}
		} else {
			opts.GenerateLine = "//go:generate go tool neomock generate " + e.pkgPath
		}
	}

	code, err := emitter.Render(iface, opts)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source([]byte(code))
	if err != nil {
		return nil, fmt.Errorf("format: %w", err)
	}
	content := formatted
	if resolved.BoilerplateFile != "" {
		bp, err := r.readBoilerplate(resolved.BoilerplateFile)
		if err != nil {
			return nil, err
		}
		content = append([]byte(bp+"\n"), formatted...)
	}

	outPath := filepath.Join(dir, filename)
	return &mockTask{
		pkgPath: e.pkgPath,
		iface:   t.name,
		path:    outPath,
		pkgDir:  dir,
		selfPkg: selfPath,
		content: content,
		force:   resolved.ForceWrite,
	}, nil
}

// scanOrphans — скан целевых каталогов на файлы с маркером neomock, которые
// больше не соответствуют ни одному сконфигурированному интерфейсу (С11).
// Только полный прогон с файлом конфигурации; иначе — notice.
func (r *runner) scanOrphans(entries []pkgEntry) error {
	full := len(r.opts.Patterns) == 0 && r.file != nil && r.opts.GoGenA == nil && !r.opts.GoGenB
	if !full {
		r.res.Notice = "orphaned-скан пропущен: работает только при полном прогоне с файлом конфигурации (С11)"
		return nil
	}
	expected := map[string]bool{}
	dirs := map[string]bool{}
	for _, t := range r.tasks {
		expected[t.path] = true
		dirs[t.pkgDir] = true
	}
	var orphans []string
	for dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			return err
		}
		for _, m := range matches {
			if expected[m] {
				continue
			}
			if !hasGeneratedMarker(m) {
				continue
			}
			orphans = append(orphans, m)
		}
	}
	sort.Strings(orphans)
	r.res.Orphaned = orphans

	if r.opts.Clean && r.mode == ModeGenerate {
		for _, p := range orphans {
			if err := os.Remove(p); err != nil {
				return fmt.Errorf("gen: --clean: %w", err)
			}
			r.res.Deleted = append(r.res.Deleted, p)
		}
	}
	return nil
}

// hasGeneratedMarker — маркер neomock в первых строках файла (С11).
func hasGeneratedMarker(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan() && i < 5; i++ {
		if strings.Contains(sc.Text(), loader.GeneratedMarker) {
			return true
		}
	}
	return false
}

func (r *runner) readBoilerplate(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.mod.Root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("boilerplate-file: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func (r *runner) writeFile(t *mockTask) error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(t.path), err)
	}
	status, err := format.Write(t.path, t.content, t.force)
	fr := FileResult{
		PkgPath: t.pkgPath,
		Iface:   t.iface,
		Path:    t.path,
		Status:  status,
		Err:     err,
	}
	r.addFile(fr)
	return err
}

func (r *runner) checkFile(t *mockTask) error {
	existing, err := os.ReadFile(t.path)
	fr := FileResult{PkgPath: t.pkgPath, Iface: t.iface, Path: t.path}
	switch {
	case os.IsNotExist(err):
		fr.Missing = true
		fr.Diff = UnifiedDiff("/dev/null", t.path, nil, t.content)
		r.mu.Lock()
		r.res.Stale++
		r.mu.Unlock()
	case err != nil:
		fr.Err = err
	default:
		if string(existing) != string(t.content) {
			fr.Diff = UnifiedDiff(t.path, t.path+" (generated)", existing, t.content)
			r.mu.Lock()
			r.res.Stale++
			r.mu.Unlock()
		}
	}
	r.addFile(fr)
	return fr.Err
}

func (r *runner) addFile(fr FileResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.res.Files = append(r.res.Files, fr)
}

func (r *runner) addWarnings(w ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.res.Warnings = append(r.res.Warnings, w...)
}

// verifyBuild — go build затронутых пакетов после записи (§4.2, default false).
func (r *runner) verifyBuild() error {
	written := map[string]bool{}
	for _, f := range r.res.Files {
		if f.Status == format.Written && f.Err == nil {
			written[f.Path] = true
		}
	}
	pkgs := map[string]bool{}
	for _, t := range r.tasks {
		if t.verify && written[t.path] {
			pkgs[t.selfPkg] = true
		}
	}
	for p := range pkgs {
		if err := GoBuild(r.workDir, p); err != nil {
			return err
		}
	}
	return nil
}

func sortedPkgKeys(m map[string]*config.PackageCfg) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedIfaceKeys(m map[string]*config.IfaceCfg) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func isUnder(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && !strings.HasPrefix(rel, "..")
}
