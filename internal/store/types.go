package store

type RepoState struct {
	Version       int                             `json:"version"`
	Repo          string                          `json:"repo,omitempty"`
	DefaultRemote string                          `json:"defaultRemote"`
	Trunk         string                          `json:"trunk"`
	UpdatedAt     string                          `json:"updatedAt"`
	Branches      map[string]BranchRecord         `json:"branches"`
	Landings      map[string]LandingRecord        `json:"landings,omitempty"`
	Verifications map[string][]VerificationRecord `json:"verifications,omitempty"`
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

type VerificationRecord struct {
	CheckType  string `json:"checkType"`
	Identifier string `json:"identifier,omitempty"`
	Passed     bool   `json:"passed"`
	HeadOID    string `json:"headOid,omitempty"`
	RecordedAt string `json:"recordedAt"`
	Note       string `json:"note,omitempty"`
	Score      *int   `json:"score,omitempty"`
}

type LandingRecord struct {
	BaseBranch     string   `json:"baseBranch"`
	SourceBranches []string `json:"sourceBranches"`
	SupersededPRs  []int    `json:"supersededPrs,omitempty"`
	SupersededAt   string   `json:"supersededAt,omitempty"`
	CreatedAt      string   `json:"createdAt"`
}

type OperationState struct {
	Version        int           `json:"version"`
	Type           string        `json:"type"`
	Repo           string        `json:"repo,omitempty"`
	RepositoryRoot string        `json:"repositoryRoot"`
	WorktreeGitDir string        `json:"worktreeGitDir"`
	OriginalBranch string        `json:"originalBranch"`
	OriginalHEAD   string        `json:"originalHead"`
	StartedAt      string        `json:"startedAt"`
	ActiveHead     string        `json:"activeHead,omitempty"`
	Active         RestackStep   `json:"active"`
	Pending        []RestackStep `json:"pending"`
}

type RestackStep struct {
	Branch             string `json:"branch"`
	Parent             string `json:"parent"`
	PreviousParentHead string `json:"previousParentHead"`
	PreviousBranchHead string `json:"previousBranchHead,omitempty"`
}
