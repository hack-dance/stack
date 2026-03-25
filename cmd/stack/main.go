package main

import (
	"os"

	"github.com/hack-dance/stack/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
