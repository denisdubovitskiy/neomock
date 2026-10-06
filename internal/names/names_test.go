package names

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sliceMethods реализует MethodSet для тестов.
type sliceMethods []string

// MethodNames возвращает имена методов.
func (s sliceMethods) MethodNames() []string { return s }

// TestHolder проверяет вычисление имени холдера.
func TestHolder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		structName string
		method     string
		want       string
	}{
		{"MockStore", "Get", "mockStoreGet"},
		{"mockfoo", "Get", "mockfooGet"},
		{"MockCache", "Get", "mockCacheGet"},
	}
	for _, tt := range tests {
		t.Run(tt.structName+"/"+tt.method, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, Holder(tt.structName, tt.method))
		})
	}
}

// TestHolderField проверяет имя поля и обход коллизий с полями мока.
func TestHolderField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		want   string
	}{
		{"Get", "get"},
		{"T", "t_1"},
		{"Map", "map_1"},
		{"Range", "range_1"},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, HolderField(tt.method))
		})
	}
}

// TestValidate проверяет все fail-fast коллизии DESIGN 3.2.6.
func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		methods []string
		history bool
		want    string // подстрока ошибки; "" — валидно
	}{
		{"ok", []string{"Get", "Put"}, true, ""},
		{"reserved Times", []string{"Times"}, true, "reserved"},
		{"reserved Then", []string{"Then"}, true, "reserved"},
		{"reserved Once", []string{"Once"}, true, "reserved"},
		{"reserved Optional", []string{"Optional"}, true, "reserved"},
		{"reserved Never", []string{"Never"}, true, "reserved"},
		{"service", []string{"NeomockVerify"}, true, "service"},
		{"history collision", []string{"GetCalls"}, true, "history: false"},
		{"history allowed when off", []string{"GetCalls"}, false, ""},
		{"When collision", []string{"Get", "WhenGet"}, true, "builder"},
		{"Run collision", []string{"Get", "RunGet"}, true, "runner"},
		{"Never collision", []string{"Get", "NeverGet"}, true, "never-rule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// act
			err := Validate(sliceMethods(tt.methods), tt.history)

			// assert
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestParamName проверяет дефолтные имена параметров.
func TestParamName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		idx  int
		in   string
		want string
	}{
		{"source name kept", 0, "ctx", "ctx"},
		{"empty becomes argN", 1, "", "arg1"},
		{"blank becomes argN", 2, "_", "arg2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, ParamName(tt.idx, tt.in))
		})
	}
}

// TestExportAndField проверяет экспорт полей и дедупликацию.
func TestExportAndField(t *testing.T) {
	t.Parallel()

	// arrange
	taken := map[string]bool{}

	// act
	first := Field(taken, "key")
	second := Field(taken, "Key")

	// assert
	assert.Equal(t, "Key", first)
	assert.Equal(t, "Key_1", second)
}
