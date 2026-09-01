package version

import "fmt"

var (
	Version = "dev"
	Commit  = "-"
	Date    = "-"
)

func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
