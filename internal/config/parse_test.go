package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalYAML — минимальный валидный конфиг.
const minimalYAML = `
version: 1
packages:
  "example.com/proj/store":
    all: true
`

// TestParse_MinimalYAML — happy path.
func TestParse_MinimalYAML(t *testing.T) {
	t.Parallel()

	// act
	f, err := Parse([]byte(minimalYAML), "yaml")

	// assert
	require.NoError(t, err)
	assert.Equal(t, 1, f.Version)
	require.Contains(t, f.Packages, "example.com/proj/store")
	assert.True(t, f.Packages["example.com/proj/store"].All)
}

// TestParse_TOMLYamlEquivalence — один набор ожиданий на оба формата (§4.1).
func TestParse_TOMLYamlEquivalence(t *testing.T) {
	t.Parallel()

	yamlSrc := `
version: 1
defaults:
  dir: "{{.InterfaceDir}}/mocks"
  strict-calls: true
  template-data:
    boilerplate-file: "lic.txt"
packages:
  "example.com/proj/store":
    interfaces:
      Cache:
        source-interface: Store
        config:
          structname: MockCache
          inpackage: false
`
	tomlSrc := `
version = 1
[defaults]
dir = "{{.InterfaceDir}}/mocks"
strict-calls = true
[defaults.template-data]
boilerplate-file = "lic.txt"
[packages."example.com/proj/store".interfaces.Cache]
source-interface = "Store"
[packages."example.com/proj/store".interfaces.Cache.config]
structname = "MockCache"
inpackage = false
`

	// act
	yf, err := Parse([]byte(yamlSrc), "yaml")
	require.NoError(t, err)
	tf, err := Parse([]byte(tomlSrc), "toml")
	require.NoError(t, err)

	// assert — оба дерева идентичны
	assert.Equal(t, yf, tf)
	ic := tf.Packages["example.com/proj/store"].Interfaces["Cache"]
	assert.Equal(t, "Store", ic.SourceInterface)
	assert.Equal(t, "MockCache", *ic.Config.Structname)
	assert.Equal(t, "lic.txt", *tf.Defaults.TemplateData.BoilerplateFile)
}

// TestParseNegatives — каждый негатив §4.2 именованным подтестом:
// парсим конфиг и ожидаем ошибку с характерным фрагментом сообщения.
func TestParseNegatives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "missing version",
			src:  "packages:\n  a:\n    all: true\n",
			want: []string{"version"},
		},
		{
			name: "wrong version",
			src:  "version: 2\npackages:\n  a:\n    all: true\n",
			want: []string{"version"},
		},
		{
			name: "two selectors",
			src:  "version: 1\npackages:\n  a:\n    all: true\n    include: X\n",
			want: []string{"ровно один селектор"},
		},
		{
			name: "no selector",
			src:  "version: 1\npackages:\n  a:\n    config:\n      dir: x\n",
			want: []string{"ровно один селектор"},
		},
		{
			name: "exclude without include",
			src:  "version: 1\npackages:\n  a:\n    exclude: X\n",
			want: []string{"exclude без include"},
		},
		{
			name: "bad source",
			src:  "version: 1\ndefaults:\n  source: weird\npackages:\n  a:\n    all: true\n",
			want: []string{"go|test|all"},
		},
		{
			name: "unknown key suggests",
			src:  "version: 1\ndefaults:\n  srict-calls: true\npackages:\n  a:\n    all: true\n",
			want: []string{"неизвестный ключ", "strict-calls"},
		},
		{
			name: "legacy unroll-variadic",
			src:  "version: 1\ndefaults:\n  unroll-variadic: true\npackages:\n  a:\n    all: true\n",
			want: []string{"удалён в ред. 2"},
		},
		{
			name: "legacy with-expecter",
			src:  "version: 1\ndefaults:\n  with-expecter: true\npackages:\n  a:\n    all: true\n",
			want: []string{"удалён в ред. 2"},
		},
		{
			name: "legacy template",
			src:  "version: 1\ndefaults:\n  template: testify\npackages:\n  a:\n    all: true\n",
			want: []string{"удалён в ред. 2"},
		},
		{
			name: "template-data closed schema",
			src:  "version: 1\ndefaults:\n  template-data:\n    custom-key: x\npackages:\n  a:\n    all: true\n",
			want: []string{"template-data"},
		},
		{
			name: "source-interface outside interfaces",
			src:  "version: 1\npackages:\n  a:\n    all: true\n    config:\n      source-interface: B\n",
			want: []string{"неизвестный ключ"},
		},
		{
			name: "bool type strict",
			src:  "version: 1\ndefaults:\n  strict-calls: \"yes\"\npackages:\n  a:\n    all: true\n",
			want: []string{"bool"},
		},
		{
			name: "unknown root key",
			src:  "version: 1\ndefaultss: {}\npackages:\n  a:\n    all: true\n",
			want: []string{"неизвестный ключ"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// act
			_, err := Parse([]byte(tt.src), "yaml")

			// assert
			require.Error(t, err)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
}

// TestParse_GenerateLine — generate-line: false парсится и доходит до Resolved;
// по умолчанию — true (шапка //go:generate эмитится).
func TestParse_GenerateLine(t *testing.T) {
	t.Parallel()

	// arrange
	src := "version: 1\ndefaults:\n  generate-line: false\npackages:\n  a:\n    all: true\n"

	// act
	f, err := Parse([]byte(src), "yaml")
	require.NoError(t, err)
	r := Resolve(LayersForFile(f, "a", "X")...)

	// assert
	require.NotNil(t, f.Defaults.GenerateLine)
	assert.False(t, *f.Defaults.GenerateLine)
	assert.False(t, r.GenerateLine)
	assert.Equal(t, "defaults", r.Sources["generate-line"])

	// по умолчанию — true
	assert.True(t, Resolve().GenerateLine)
}
