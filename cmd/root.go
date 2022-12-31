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
		url := args[0]
		s := grabber.NewSite(url)

		// ranges parsing
		rngs, err := ranges.Parse(args[1])
		if err != nil {
			panic(err)
		}

		// Fetch series title
		title := s.Title()
		// Fetch chapters
		chapters := s.FetchChapters(cmd.Flag("language").Value.String())

		// Filter and sort ranges
		chapters = chapters.FilterRanges(rngs)

		// loop chapters to retrieve pages
		for _, chap := range chapters {
			chapter := s.FetchChapter(chap)
			fmt.Printf("Working on %s %s\n", color.New(color.FgGreen).Sprint(title), chapter.Title)

			files, err := downloader.FetchChapter(chapter)
			if err != nil {
				panic(err)
			}

			filename := fmt.Sprintf("%s %s.cbz", title, chapter.Title)
			color.Green("- saving file %s", filename)
			err = packer.ArchiveCBZ(filename, files)
			if err != nil {
				panic(err)
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

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.manga-downloader.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.Flags().StringP("language", "l", "", "Only download the specified language")
}
