// Package gen — конвейер генерации: конфигурация → загрузка пакетов →
// селекция интерфейсов → эмиссия → запись/сравнение (DESIGN §3.1, §5).
// Используется командами generate и check.
package gen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ModuleInfo — данные текущего модуля (placement rules, §3.3).
type ModuleInfo struct {
	// Root — каталог модуля (где go.mod).
	Root string
	// Path — import path модуля.
	Path string
}

// DetectModule определяет текущий модуль по каталогу workDir.
func DetectModule(workDir string) (*ModuleInfo, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}|{{.Path}}")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gen: не внутри go-модуля (go list -m): %w", err)
	}
	// В go.work `go list -m` печатает все модули воркспейса — берём тот,
	// что содержит workDir (наиболее глубокий совпадающий префикс).
	best := ""
	var info *ModuleInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		dir, path, ok := strings.Cut(line, "|")
		if !ok || dir == "" || path == "" {
			continue
		}
		if (dir == workDir || strings.HasPrefix(workDir, dir+string(os.PathSeparator))) && len(dir) > len(best) {
			best, info = dir, &ModuleInfo{Root: dir, Path: path}
		}
	}
	if info == nil {
		return nil, fmt.Errorf("gen: не удалось определить модуль в %s", workDir)
	}
	return info, nil
}

// ImportPathOf определяет import path пакета в каталоге dir (режимы А/Б).
func ImportPathOf(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gen: go list в %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ExpandPattern раскрывает паттерн пакетов (./path/..., import/path/...)
// в список import path через go list.
func ExpandPattern(workDir, pattern string) ([]string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", pattern)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gen: go list %s: %w", pattern, err)
	}
	var out2 []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			out2 = append(out2, line)
		}
	}
	return out2, nil
}

// GoBuild выполняет go build пакета (verify-build, §4.2).
func GoBuild(workDir, pkgPath string) error {
	cmd := exec.Command("go", "build", pkgPath)
	cmd.Dir = workDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gen: go build %s: %w\n%s", pkgPath, err, out)
	}
	return nil
}
