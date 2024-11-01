package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/tcnksm/go-latest"
)

var (
	// Tag is the git tag of the current build
	Tag = "develop"
	// Version is the version of the current build
	Version = "develop"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Shows the version of the application",
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("manga-downloader - manga volumes downloading tool")
		slog.Info(fmt.Sprintf("version: %s (%s)", Version, Tag))

		vcheck := &latest.GithubTag{
			Owner:             "voxelost",
			Repository:        "manga-downloader",
			FixVersionStrFunc: latest.DeleteFrontV(),
		}

		res, err := latest.Check(vcheck, Tag)
		if err != nil {
			slog.Error(fmt.Sprintf("error checking for updates: %v", err))
			return
		}
		if res.Outdated {
			slog.Warn(
				fmt.Sprintf("app is outdated - download latest (%s) from: https://github.com/voxelost/manga-downloader/releases/tag/v%s",
					res.Current,
					res.Current,
				))
		} else {
			slog.Info("app is up to date.")
		}
	},
}
