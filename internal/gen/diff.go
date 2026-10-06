package gen

import (
	"fmt"
	"strings"
)

// UnifiedDiff — unified diff между old и new (контекст 3 строки).
// Реализация: отбрасывание общего префикса/суффикса + LCS по середине.
// Файлы моков невелики, O(n·m) памяти достаточно.
func UnifiedDiff(oldName, newName string, old, new []byte) string {
	a := splitLines(string(old))
	b := splitLines(string(new))

	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}
	aMid, bMid := a[start:endA], b[start:endB]

	ops := lcsOps(aMid, bMid)

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)

	const ctx = 3
	// помечаем ops для показа: изменения ± ctx
	show := make([]bool, len(ops))
	for k, o := range ops {
		if o.kind == opKeep {
			continue
		}
		for c := max(k-ctx, 0); c <= min(k+ctx, len(ops)-1); c++ {
			show[c] = true
		}
	}

	lineA, lineB := start, start
	k := 0
	for k < len(ops) {
		if !show[k] {
			if ops[k].kind == opKeep {
				lineA++
				lineB++
			}
			k++
			continue
		}
		// границы ханка
		hunkStart := k
		for k < len(ops) && show[k] {
			k++
		}
		hunk := ops[hunkStart:k]
		var oldN, newN int
		for _, o := range hunk {
			if o.kind != opAdd {
				oldN++
			}
			if o.kind != opDel {
				newN++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", lineA+1, oldN, lineB+1, newN)
		for _, o := range hunk {
			switch o.kind {
			case opKeep:
				fmt.Fprintf(&out, " %s\n", o.line)
				lineA++
				lineB++
			case opDel:
				fmt.Fprintf(&out, "-%s\n", o.line)
				lineA++
			case opAdd:
				fmt.Fprintf(&out, "+%s\n", o.line)
				lineB++
			}
		}
	}
	return out.String()
}

const (
	opKeep = iota
	opDel
	opAdd
)

type op struct {
	kind int
	line string
}

// lcsOps — выравнивание двух блоков через LCS.
func lcsOps(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{opKeep, a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, op{opDel, a[i]})
			i++
		default:
			ops = append(ops, op{opAdd, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{opDel, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{opAdd, b[j]})
	}
	return ops
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
