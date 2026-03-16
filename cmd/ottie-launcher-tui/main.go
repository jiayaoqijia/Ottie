package main

import (
	"fmt"
	"os"

	"github.com/jiayaoqijia/ottie/cmd/ottie-launcher-tui/internal/ui"
)

func main() {
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
