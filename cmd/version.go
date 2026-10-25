// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/elboletaire/manga-downloader/update"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// Tag is the git tag of the current build
	Tag = "develop"
	// Version is the version of the current build
	Version = "develop"
)

const (
	kofiURL = "https://ko-fi.com/elboletaire"
	// noUpdateCheckEnv turns the update check off when set to 1
	noUpdateCheckEnv = "MANGA_DOWNLOADER_NO_UPDATE_CHECK"
)

// updateSource is where the update check asks for the latest release, a test seam
var updateSource = update.Default

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Shows the version of the application",
	Run: func(cmd *cobra.Command, args []string) {
		showVersion()
	},
}

// showVersion prints the current version and checks for a newer release.
// Shared by `version` and the root `--version` flag so both produce identical
// output.
func showVersion() {
	fmt.Printf("%s - Manga volumes downloading tool\n", color.YellowString("Manga Downloader"))
	fmt.Printf("All Rights Reserved © 2023-2026 %s\n", color.HiBlackString("Òscar Casajuana Alonso"))
	version := color.MagentaString("%s (%s)", Version, Tag)
	if os.Getenv(noUpdateCheckEnv) == "1" {
		fmt.Printf("Version: %s\n", version)
	} else {
		fmt.Printf("Version: %s - %s", version, updateStatus(context.Background(), Tag, updateSource))
	}
	fmt.Println(color.HiBlackString("Enjoying it? Buy me a coffee: %s", kofiURL))
}

// updateStatus says whether the current build is outdated, as the line after its
// version. A dev build makes no request, as it has no release to compare with.
func updateStatus(ctx context.Context, current string, src update.Source) string {
	if !update.IsRelease(current) {
		return "Development build, no release to compare with.\n"
	}
	body, err := src.Latest(ctx, "cli", current)
	if err != nil {
		return fmt.Sprintf("Error checking for updates: %s\n", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return fmt.Sprintf("Error checking for updates: %s\n", err)
	}
	if release.TagName == "" {
		return "Error checking for updates: the latest release has no tag\n"
	}
	newer, err := update.Newer(current, release.TagName)
	if err != nil {
		return fmt.Sprintf("Error checking for updates: %s\n", err)
	}
	if !newer {
		return fmt.Sprintf("%s\n", color.GreenString("App is up to date."))
	}

	return fmt.Sprintf(
		"%s Download latest (%s) from:\n%s\n",
		color.HiRedString("App is outdated."),
		color.RedString(strings.TrimPrefix(release.TagName, "v")),
		"https://github.com/elboletaire/manga-downloader/releases/tag/"+release.TagName,
	)
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
