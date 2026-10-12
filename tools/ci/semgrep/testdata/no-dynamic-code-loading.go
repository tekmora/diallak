//go:build semgrep_fixture

package fixture

// ruleid: no-dynamic-code-loading
import "plugin"

// ok: no-dynamic-code-loading
import "reflect"

func load(path string) (plugin.Symbol, error) {
	// ruleid: no-dynamic-code-loading
	p, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}
	return p.Lookup("Handler")
}

func build(t reflect.Type, fn func([]reflect.Value) []reflect.Value) reflect.Value {
	// ruleid: no-dynamic-code-loading
	return reflect.MakeFunc(t, fn)
}

func value(x any) reflect.Value {
	// ok: no-dynamic-code-loading
	return reflect.ValueOf(x)
}
