package main

import (
	"devpod/cigno/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
