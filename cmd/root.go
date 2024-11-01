package cmd

import (
	"log/slog"
	"os"
	"strings"

	"github.com/ivanpirog/coloredcobra"
	"github.com/spf13/cobra"
	"github.com/voxelost/manga-downloader/grabber"
	"github.com/voxelost/manga-downloader/packer"
)

var settings grabber.Settings

func Execute() {
	var RootCmd = &cobra.Command{
		Use:   CobraUse,
		Short: CobraShort,

		Long:    CobraLong,
		Example: CobraExample,
		Args:    cobra.MinimumNArgs(1),
		Run:     Handler,
	}

	RootCmd.Flags().BoolVarP(&settings.Bundle, "bundle", "b", false, "bundle all specified chapters into a single file")
	RootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Chapters, "concurrency", "c", 5, "number of concurrent chapter downloads, hard-limited to 5")
	RootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Pages, "concurrency-pages", "C", 10, "number of concurrent page downloads, hard-limited to 10")
	RootCmd.Flags().StringVarP(&settings.Language, "language", "l", "", "only download the specified language")
	RootCmd.Flags().StringVarP(&settings.FilenameTemplate, "filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename")
	RootCmd.AddCommand(versionCmd)

	// set as persistent, so version command does not complain about the -o flag set via docker
	RootCmd.PersistentFlags().StringVarP(&settings.OutputDir, "output-dir", "o", "./", "output directory for the downloaded files")

	coloredcobra.Init(&coloredcobra.Config{
		RootCmd:       RootCmd,
		Headings:      coloredcobra.HiCyan + coloredcobra.Bold,
		Commands:      coloredcobra.HiYellow + coloredcobra.Bold,
		Aliases:       coloredcobra.Bold + coloredcobra.Italic,
		CmdShortDescr: coloredcobra.HiRed,
		ExecName:      coloredcobra.Bold,
		Flags:         coloredcobra.Bold,
		FlagsDescr:    coloredcobra.HiMagenta,
		FlagsDataType: coloredcobra.Italic,
	})

	if err := RootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func getRangesArg(args []string) string {
	if len(args) == 1 {
		return ""
	}

	if strings.HasPrefix(args[0], "http") {
		return args[1]
	}

	return args[0]
}

func getURLArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	if strings.HasPrefix(args[0], "http") {
		return args[0]
	}

	return args[1]
}
