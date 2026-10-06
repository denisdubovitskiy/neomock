package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

// TestResolve_Priority — приоритет слоёв: builtins → defaults → for-package →
// for-interface → go:generate (§4.1).
func TestResolve_Priority(t *testing.T) {
	t.Parallel()

	// arrange
	f := &File{
		Version: 1,
		Defaults: &Settings{
			Dir:     sp("defaults-dir"),
			Pkgname: sp("defaults-pkg"),
		},
		Packages: map[string]*PackageCfg{
			"p": {
				All:    true,
				Config: &Settings{Dir: sp("pkg-dir"), StrictCalls: bp(true)},
				Interfaces: map[string]*IfaceCfg{
					"I": {Config: &Settings{Dir: sp("iface-dir")}},
				},
			},
		},
	}

	// act
	layers := LayersForFile(f, "p", "I")
	gogen := Layer{Name: "go:generate", S: &Settings{Filename: sp("gen_file.go")}}
	r := Resolve(append(layers, gogen)...)

	// assert
	assert.Equal(t, "iface-dir", r.Dir)
	assert.Equal(t, "for-interface I", r.Sources["dir"])
	assert.Equal(t, "defaults-pkg", r.Pkgname)
	assert.Equal(t, "defaults", r.Sources["pkgname"])
	assert.True(t, r.StrictCalls)
	assert.Equal(t, "for-package p", r.Sources["strict-calls"])
	assert.Equal(t, "gen_file.go", r.Filename)
	assert.Equal(t, "go:generate", r.Sources["filename"])
	// builtins не перекрывались:
	assert.Equal(t, "go", r.Source)
	assert.Equal(t, "builtins", r.Sources["source"])
}

// TestResolve_Builtins — значения по умолчанию §4.2 без конфига.
func TestResolve_Builtins(t *testing.T) {
	t.Parallel()

	// act
	r := Resolve()

	// assert
	assert.Equal(t, "{{.InterfaceDir}}/mocks", r.Dir)
	assert.Equal(t, "{{.InterfaceSnake}}_mock.go", r.Filename)
	assert.Equal(t, "mocks", r.Pkgname)
	assert.Equal(t, "{{.Mock}}{{.InterfaceName}}", r.Structname)
	assert.Equal(t, "go", r.Source)
	assert.False(t, r.InPackage)
	assert.True(t, r.ForceWrite)
	assert.False(t, r.VerifyBuild)
	assert.False(t, r.StrictCalls)
	assert.True(t, r.History)
}

// TestExplain — форма вывода explain: значение + источник (§4.7).
func TestExplain(t *testing.T) {
	t.Parallel()

	// arrange
	r := Resolve(
		Layer{Name: "for-interface Cache", S: &Settings{Structname: sp("MockCache")}},
	)

	// act
	out := r.Explain("example.com/proj/cache.Cache")

	// assert
	assert.Contains(t, out, "interface: example.com/proj/cache.Cache")
	assert.Contains(t, out, "structname:")
	assert.Contains(t, out, "MockCache")
	assert.Contains(t, out, "(for-interface Cache)")
	assert.Contains(t, out, "(builtins)")
}

// TestMergeSettings — overlay перекрывает base только заданными полями.
func TestMergeSettings(t *testing.T) {
	t.Parallel()

	// arrange
	base := BuiltinDefaults()
	overlay := &Settings{Dir: sp("x"), History: bp(false)}

	// act
	got := MergeSettings(base, overlay)

	// assert
	assert.Equal(t, "x", *got.Dir)
	assert.False(t, *got.History)
	assert.Equal(t, "mocks", *got.Pkgname, "не заданное поле осталось из base")
}
