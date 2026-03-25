package app

import (
	"fmt"
	"os"

	"github.com/hack-dance/stack/internal/cmd"
	stackruntime "github.com/hack-dance/stack/internal/runtime"
)

func Run(args []string) int {
	runtime := stackruntime.New()
	root := cmd.NewRootCommand(runtime)
	root.SetArgs(args)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}
