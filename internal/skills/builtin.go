package skills

import "embed"

//go:embed builtins archives
var builtinFiles embed.FS

func BuiltinRegistry() (*Registry, error) {
	registry, err := LoadFS(builtinFiles, "builtins")
	if err != nil {
		return nil, err
	}
	history, err := LoadFS(builtinFiles, "archives/1.0.0")
	if err != nil {
		return nil, err
	}
	if err := registry.mergeHistory(history); err != nil {
		return nil, err
	}
	return registry, nil
}
