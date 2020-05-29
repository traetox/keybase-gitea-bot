package main

import (
	"fmt"
	"os"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

func mainInner() int {
	opts, exitCode, ok := LoadOptions()
	if !ok {
		return exitCode
	}

	bs := NewBotServer(*opts)
	if err := bs.Go(); err != nil {
		fmt.Printf("error running chat loop: %s\n", err)
		return 3
	}

	return 0
}
