/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "manga-downloader",
	Short: "Helps you download manga from websites to CBZ files",
	Long: `With manga-downloader you can easily convert web based mangas
to CBZ files.`,
	Example: `manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10`,
	Args:    cobra.ExactArgs(2),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		s := grabber.NewSite(args[0])
		if s == nil {
			fmt.Println("Site not recognised")
			os.Exit(1)
		}

		// ranges parsing
		rngs, err := ranges.Parse(args[1])
		if err != nil {
			panic(err)
		}

		// language flag (if any)
		language := cmd.Flag("language").Value.String()

		// fetch series title
		title := s.GetTitle(language)

		// fetch all chapters
		chapters := s.FetchChapters(language)

		// sort and filter specified ranges
		chapters = chapters.FilterRanges(rngs)

		// loop chapters to retrieve pages
		for _, chap := range chapters {
			chapter := s.FetchChapter(chap)
			fmt.Printf("%s %s:\n", color.New(color.FgGreen).Sprint(title), chapter.Title)

			files, err := downloader.FetchChapter(s, chapter)
			if err != nil {
				panic(err)
			}

			filename := fmt.Sprintf("%s %s.cbz", title, chapter.Title)
			color.Green("- saving file %s", filename)
			err = packer.ArchiveCBZ(filename, files)
			if err != nil {
				color.Red("- error saving file %s: %s", filename, err.Error())
			}
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// init sets the flags for the root command
func init() {
	rootCmd.Flags().StringP("language", "l", "", "Only download the specified language")
}
