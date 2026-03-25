package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	stackgit "github.com/hack-dance/stack/internal/git"
)

const stateVersion = 1

type Paths struct {
	Root      string
	GitDir    string
	CommonDir string
	StateFile string
	OpFile    string
	LockDir   string
}

type Store struct {
	git *stackgit.Client
}

func New(git *stackgit.Client) *Store {
	return &Store{git: git}
}

func (s *Store) ResolvePaths(ctx context.Context) (Paths, error) {
	repoPaths, err := s.git.RepoPaths(ctx)
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		Root:      repoPaths.Root,
		GitDir:    repoPaths.GitDir,
		CommonDir: repoPaths.CommonDir,
		StateFile: filepath.Join(repoPaths.CommonDir, "stack", "state.json"),
		OpFile:    filepath.Join(repoPaths.GitDir, "stack", "op.json"),
		LockDir:   filepath.Join(repoPaths.CommonDir, "stack", "lock"),
	}, nil
}

func (s *Store) InitState(ctx context.Context, trunk string, remote string, repo string) (RepoState, error) {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return RepoState{}, err
	}

	if err := os.MkdirAll(filepath.Dir(paths.StateFile), 0o755); err != nil {
		return RepoState{}, err
	}

	state := RepoState{
		Version:       stateVersion,
		Repo:          repo,
		DefaultRemote: remote,
		Trunk:         trunk,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Branches:      map[string]BranchRecord{},
	}

	if err := s.writeJSON(paths.StateFile, state); err != nil {
		return RepoState{}, err
	}

	return state, nil
}

func (s *Store) ReadState(ctx context.Context) (RepoState, error) {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return RepoState{}, err
	}

	data, err := os.ReadFile(paths.StateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepoState{}, fmt.Errorf("stack is not initialized; run `stack init`")
		}
		return RepoState{}, err
	}

	var state RepoState
	if err := json.Unmarshal(data, &state); err != nil {
		return RepoState{}, err
	}

	if state.Branches == nil {
		state.Branches = map[string]BranchRecord{}
	}

	return state, nil
}

func (s *Store) WriteState(ctx context.Context, state RepoState) error {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(paths.StateFile), 0o755); err != nil {
		return err
	}

	state.Version = stateVersion
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	return s.writeJSON(paths.StateFile, state)
}

func (s *Store) ReadOperation(ctx context.Context) (OperationState, error) {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return OperationState{}, err
	}

	data, err := os.ReadFile(paths.OpFile)
	if err != nil {
		return OperationState{}, err
	}

	var operation OperationState
	if err := json.Unmarshal(data, &operation); err != nil {
		return OperationState{}, err
	}

	return operation, nil
}

func (s *Store) WriteOperation(ctx context.Context, operation OperationState) error {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(paths.OpFile), 0o755); err != nil {
		return err
	}

	operation.Version = stateVersion
	return s.writeJSON(paths.OpFile, operation)
}

func (s *Store) ClearOperation(ctx context.Context) error {
	paths, err := s.ResolvePaths(ctx)
	if err != nil {
		return err
	}

	if err := os.Remove(paths.OpFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (s *Store) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	temp := fmt.Sprintf("%s.tmp", path)
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}

	return os.Rename(temp, path)
}
