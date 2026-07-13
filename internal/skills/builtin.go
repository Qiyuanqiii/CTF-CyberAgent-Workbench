package skills

import "embed"

//go:embed builtins
var builtinFiles embed.FS

func BuiltinRegistry() (*Registry, error) {
	return LoadFS(builtinFiles, "builtins")
}
