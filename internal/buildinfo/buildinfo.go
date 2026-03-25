package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func Summary() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
