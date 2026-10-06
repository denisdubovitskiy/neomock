// Package format доводит сгенерированный код до записи: go/format.Source
// поверх готового файла (goimports не нужен — импорты считает Registry)
// и атомарная запись с пропуском при совпадении байтов (DESIGN §5: нетронутый
// mtime — нет пересборки Go).
package format

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
)

// Status — итог записи сгенерированного файла.
type Status int

const (
	// Written — файл записан (новый или изменился).
	// Нумерация с 1: нулевое значение Status не должно читаться как «записан».
	Written Status = iota + 1
	// Unchanged — сгенерированные байты совпали с существующими: файл не
	// тронут (mtime сохранён — пересборки Go нет).
	Unchanged
	// SkippedManual — файл отличается, а force-write выключен: ручная правка
	// не затёрта; расхождение видно в diff команды check.
	SkippedManual
)

// String возвращает читаемое имя статуса для журнала.
func (s Status) String() string {
	switch s {
	case Written:
		return "written"
	case Unchanged:
		return "unchanged"
	case SkippedManual:
		return "skipped (manual edits preserved)"
	default:
		return fmt.Sprintf("unknown status %d", int(s))
	}
}

// Source форматирует сгенерированный код через go/format.Source.
func Source(src []byte) ([]byte, error) {
	out, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("format: %w", err)
	}
	return out, nil
}

// Write записывает formatted в path по правилам §5:
//   - байты совпали с существующим файлом → Unchanged, файл не тронут;
//   - отличаются и force → атомарная перезапись (tmp+rename в том же каталоге);
//   - отличаются и !force → SkippedManual (ручные правки сохраняются).
//
// Файл отсутствует → записывается всегда (вне зависимости от force).
func Write(path string, formatted []byte, force bool) (Status, error) {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if bytes.Equal(existing, formatted) {
			return Unchanged, nil
		}
		if !force {
			return SkippedManual, nil
		}
	case os.IsNotExist(err):
		// Новый файл — пишем безусловно.
	default:
		return Written, fmt.Errorf("format: read %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return Written, fmt.Errorf("format: temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op после успешного rename

	// Права сохраняем от существующего файла, для нового — 0o644.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if _, err := tmp.Write(formatted); err != nil {
		_ = tmp.Close()
		return Written, fmt.Errorf("format: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return Written, fmt.Errorf("format: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return Written, fmt.Errorf("format: chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return Written, fmt.Errorf("format: rename %s -> %s: %w", tmpName, path, err)
	}
	return Written, nil
}
