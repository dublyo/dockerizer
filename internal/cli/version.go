package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// VersionOutput is the JSON output for version command
type VersionOutput struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	GitCommit string `json:"gitCommit"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information including build time and git commit.`,
	Run:   runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	if jsonOut {
		output := VersionOutput{
			Version:   Version,
			BuildTime: BuildTime,
			GitCommit: GitCommit,
			GoVersion: runtime.Version(),
			Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(output)
		return
	}

	fmt.Printf("dockerizer %s\n", Version)
	fmt.Printf("  Build time: %s\n", BuildTime)
	fmt.Printf("  Git commit: %s\n", GitCommit)
	fmt.Printf("  Go version: %s\n", runtime.Version())
	fmt.Printf("  Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
