package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fileexists <path>")
		os.Exit(1)
	}
	if _, err := os.Stat(os.Args[1]); err != nil {
		os.Exit(1)
	}
}
