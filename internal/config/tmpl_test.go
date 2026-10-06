package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderTemplate_Vars — все переменные §4.2 доступны.
func TestRenderTemplate_Vars(t *testing.T) {
	t.Parallel()

	vars := TemplateVars{
		InterfaceName:        "ResponseWriter",
		InterfaceSnake:       Snake("ResponseWriter"),
		Mock:                 "Mock",
		StructName:           "MockResponseWriter",
		SrcPackageName:       "net/http",
		SrcPackagePath:       "net/http",
		InterfaceDir:         "/go/src/net/http",
		InterfaceDirRelative: "net/http",
		InterfaceFile:        "/go/src/net/http/server.go",
	}

	tests := []struct{ tmpl, want string }{
		{"{{.InterfaceName}}", "ResponseWriter"},
		{"{{.InterfaceSnake}}", "response_writer"},
		{"{{.Mock}}{{.InterfaceName}}", "MockResponseWriter"},
		{"{{.StructName}}", "MockResponseWriter"},
		{"{{.SrcPackageName}}", "net/http"},
		{"{{.InterfaceDir}}/mocks", "/go/src/net/http/mocks"},
		{"{{.InterfaceDirRelative}}", "net/http"},
		{"{{base .InterfaceFile}}", "server.go"},
		{"{{dir .InterfaceFile}}", "/go/src/net/http"},
		{"{{lower .InterfaceName}}", "responsewriter"},
		{"{{upper .InterfaceName}}", "RESPONSEWRITER"},
		{"{{title .InterfaceName}}", "ResponseWriter"},
		{"{{snake .InterfaceName}}_mock.go", "response_writer_mock.go"},
		{`{{replaceAll "/" "_" .SrcPackagePath}}`, "net_http"},
	}
	for _, tt := range tests {
		t.Run(tt.tmpl, func(t *testing.T) {
			t.Parallel()

			// act
			got, err := RenderTemplate(tt.tmpl, vars)

			// assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRenderTemplate_UnknownVar — неизвестная переменная — ошибка (один проход).
func TestRenderTemplate_UnknownVar(t *testing.T) {
	t.Parallel()

	// act
	_, err := RenderTemplate("{{.Nope}}", TemplateVars{})

	// assert
	require.Error(t, err)
}

// TestSnake — таблица snake_case.
func TestSnake(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"Store", "store"},
		{"ResponseWriter", "response_writer"},
		{"HTTPServer", "http_server"},
		{"IDKey", "id_key"},
		{"A", "a"},
		{"lowerCamel", "lower_camel"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in+"→"+tt.want, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, Snake(tt.in))
		})
	}
}

// TestMockPrefix — Mock для экспортированного, mock для приватного (§3.3).
func TestMockPrefix(t *testing.T) {
	t.Parallel()

	// assert
	assert.Equal(t, "Mock", MockPrefix("Store"))
	assert.Equal(t, "mock", MockPrefix("privateCache"))
}
