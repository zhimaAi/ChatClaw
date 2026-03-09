package main

import (
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 || strings.TrimSpace(os.Args[1]) == "" {
		os.Exit(1)
	}
}
