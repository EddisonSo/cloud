package cli

import (
	"fmt"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

// Version is the build version, set via -ldflags
// "-X eddisonso.com/edd-cli/internal/cli.Version=<tag>" by the release
// workflow. Local/dev builds report "dev".
var Version = "dev"

func init() { register(command{name: "version", run: cmdVersion}) }

func cmdVersion(_ *eddsdk.Client, _ string, _ []string) error {
	fmt.Println(Version)
	return nil
}
