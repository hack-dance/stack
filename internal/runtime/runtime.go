package runtime

import (
	"context"
	"os"

	charmlog "github.com/charmbracelet/log"
	stackgit "github.com/hack-dance/stack/internal/git"
	stackgh "github.com/hack-dance/stack/internal/github"
	"github.com/hack-dance/stack/internal/store"
)

type Runtime struct {
	Context context.Context
	Git     *stackgit.Client
	GitHub  *stackgh.Client
	Store   *store.Store
	Logger  *charmlog.Logger
}

func New() *Runtime {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	gitClient := stackgit.NewClient(cwd)

	return &Runtime{
		Context: context.Background(),
		Git:     gitClient,
		GitHub:  stackgh.NewClient(cwd),
		Store:   store.New(gitClient),
		Logger:  charmlog.NewWithOptions(os.Stderr, charmlog.Options{Prefix: "stack"}),
	}
}
