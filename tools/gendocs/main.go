package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hack-dance/stack/internal/cmd"
	stackruntime "github.com/hack-dance/stack/internal/runtime"
	"github.com/spf13/cobra/doc"
)

func main() {
	runtime := stackruntime.New()
	root := cmd.NewRootCommand(runtime)
	root.DisableAutoGenTag = true

	target := filepath.Join("docs", "cli")
	if err := os.MkdirAll(target, 0o755); err != nil {
		fail(err)
	}

	linkHandler := func(name string) string {
		return name
	}

	filePrepender := func(filename string) string {
		base := filepath.Base(filename)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		return fmt.Sprintf("# %s\n\nGenerated from the current Cobra command tree.\n\n", name)
	}

	if err := doc.GenMarkdownTreeCustom(root, target, filePrepender, linkHandler); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
