package store

type RepoState struct {
	Version       int                     `json:"version"`
	Repo          string                  `json:"repo,omitempty"`
	DefaultRemote string                  `json:"defaultRemote"`
	Trunk         string                  `json:"trunk"`
	UpdatedAt     string                  `json:"updatedAt"`
	Branches      map[string]BranchRecord `json:"branches"`
}

type BranchRecord struct {
	ParentBranch string          `json:"parentBranch"`
	RemoteName   string          `json:"remoteName,omitempty"`
	PR           PullRequest     `json:"pr,omitempty"`
	Restack      RestackMetadata `json:"restack,omitempty"`
}

type PullRequest struct {
	ID              string `json:"id,omitempty"`
	Number          int    `json:"number,omitempty"`
	URL             string `json:"url,omitempty"`
	Repo            string `json:"repo,omitempty"`
	HeadRefName     string `json:"headRefName,omitempty"`
	BaseRefName     string `json:"baseRefName,omitempty"`
	LastSeenHeadOID string `json:"lastSeenHeadOid,omitempty"`
	LastSeenBaseOID string `json:"lastSeenBaseOid,omitempty"`
	State           string `json:"state,omitempty"`
	IsDraft         bool   `json:"isDraft,omitempty"`
}

type RestackMetadata struct {
	LastParentHeadOID string `json:"lastParentHeadOid,omitempty"`
	LastRestackedAt   string `json:"lastRestackedAt,omitempty"`
}

type OperationState struct {
	Version        int           `json:"version"`
	Type           string        `json:"type"`
	RepositoryRoot string        `json:"repositoryRoot"`
	WorktreeGitDir string        `json:"worktreeGitDir"`
	OriginalHEAD   string        `json:"originalHead"`
	StartedAt      string        `json:"startedAt"`
	Active         RestackStep   `json:"active"`
	Pending        []RestackStep `json:"pending"`
}

type RestackStep struct {
	Branch             string `json:"branch"`
	Parent             string `json:"parent"`
	PreviousParentHead string `json:"previousParentHead"`
}
