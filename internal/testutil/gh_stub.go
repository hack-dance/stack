package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

const ghStubScript = `#!/usr/bin/env python3
import json
import os
import sys

state_path = os.environ["STACK_TEST_GH_STATE"]
log_path = os.environ["STACK_TEST_GH_LOG"]

with open(log_path, "a", encoding="utf-8") as handle:
    handle.write(" ".join(sys.argv[1:]) + "\n")

if os.path.exists(state_path):
    with open(state_path, "r", encoding="utf-8") as handle:
        state = json.load(handle)
else:
    state = {"repo": {}, "prs": {}, "next_number": 1}

def save():
    with open(state_path, "w", encoding="utf-8") as handle:
        json.dump(state, handle)

def find_flag(args, name, default=""):
    for index, value in enumerate(args):
        if value == name and index + 1 < len(args):
            return args[index + 1]
    return default

args = sys.argv[1:]
if args[:2] == ["repo", "view"]:
    print(json.dumps(state.get("repo", {})))
    sys.exit(0)

if args[:2] == ["pr", "list"]:
    head = find_flag(args, "--head", "")
    result = []
    for pr in state.get("prs", {}).values():
        if pr.get("headRefName") == head:
            result.append(pr)
    print(json.dumps(result))
    sys.exit(0)

if args[:2] == ["pr", "view"]:
    number = args[2]
    pr = state.get("prs", {}).get(str(number))
    if pr is None:
        print(f"unknown pr {number}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(pr))
    sys.exit(0)

if args[:2] == ["pr", "create"]:
    base = find_flag(args, "--base", "")
    head = find_flag(args, "--head", "")
    title = find_flag(args, "--title", head)
    body = find_flag(args, "--body", "")
    draft = "--draft" in args
    number = state.get("next_number", 1)
    repo_name = state.get("repo", {}).get("nameWithOwner", "hack-dance/stack")
    pr = {
        "id": f"PR_{number}",
        "number": number,
        "url": f"https://example.com/{repo_name}/pull/{number}",
        "repo": repo_name,
        "headRefName": head,
        "baseRefName": base,
        "lastSeenHeadOid": "",
        "lastSeenBaseOid": "",
        "state": "OPEN",
        "isDraft": draft,
        "title": title,
        "body": body,
    }
    state.setdefault("prs", {})[str(number)] = pr
    state["next_number"] = number + 1
    save()
    print(pr["url"])
    sys.exit(0)

if args[:2] == ["pr", "edit"]:
    number = args[2]
    pr = state.get("prs", {}).get(str(number))
    if pr is None:
        print(f"unknown pr {number}", file=sys.stderr)
        sys.exit(1)
    base = find_flag(args, "--base", "")
    if base:
        pr["baseRefName"] = base
        state["prs"][str(number)] = pr
        save()
    sys.exit(0)

if args[:2] == ["pr", "merge"]:
    number = args[2]
    pr = state.get("prs", {}).get(str(number))
    if pr is None:
        print(f"unknown pr {number}", file=sys.stderr)
        sys.exit(1)
    pr["mergedWithHead"] = find_flag(args, "--match-head-commit", "")
    state["prs"][str(number)] = pr
    save()
    sys.exit(0)

print("unsupported gh invocation", " ".join(args), file=sys.stderr)
sys.exit(1)
`

type GHStub struct {
	Dir       string
	StatePath string
	LogPath   string
}

func SetupGHStub(t *testing.T, repoName string, defaultBranch string) GHStub {
	t.Helper()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	logPath := filepath.Join(dir, "gh.log")
	scriptPath := filepath.Join(dir, "gh")

	state := []byte(`{
  "repo": {
    "nameWithOwner": "` + repoName + `",
    "url": "https://github.com/` + repoName + `",
    "defaultBranchRef": {
      "name": "` + defaultBranch + `"
    }
  },
  "prs": {},
  "next_number": 1
}
`)
	if err := os.WriteFile(statePath, state, 0o644); err != nil {
		t.Fatalf("write gh state: %v", err)
	}
	if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write gh log: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(ghStubScript), 0o755); err != nil {
		t.Fatalf("write gh stub: %v", err)
	}

	return GHStub{
		Dir:       dir,
		StatePath: statePath,
		LogPath:   logPath,
	}
}
