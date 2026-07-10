package main

import (
	"os"

	"cyberagent-workbench/internal/app"
)

func main() {
	os.Exit(app.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
