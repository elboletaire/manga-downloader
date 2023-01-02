/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cc "github.com/ivanpirog/coloredcobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "manga-downloader [flags] [url] [ranges]",
	Short: "Helps you download mangas from websites to CBZ files",

	Long: `With manga-downloader you can easily convert/download
web based mangas to CBZ files.

You only need to specify the URL of the manga and the
chapters you want to download as a range.

Note the URL must be of the index of the manga, not a
single chapter.`,
	Example: strings.ReplaceAll(`  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10

Would download chapters 1 to 10 of Dr. Stone from inmanga.com

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10,12,15-20

Would download chapters 1 to 10, 12 and 15 to 20 of Dr. Stone from inmanga.com

  manga-downloader --language es https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

Would download chapters 10 to 20 of Black Clover from mangadex.org in Spanish`, "manga-downloader", color.YellowString("manga-downloader")),
	Args: cobra.ExactArgs(2),
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

		if len(chapters) == 0 {
			color.Yellow("No chapters found for the specified ranges")
			os.Exit(0)
		}

		// loop chapters to retrieve pages
		for _, chap := range chapters {
			chapter := s.FetchChapter(chap)
			chapter.Title = strings.TrimSpace(chapter.Title)
			fmt.Printf("%s %s:\n", color.CyanString(title), color.BlackString(chapter.Title))

			files, err := downloader.FetchChapter(s, chapter)
			if err != nil {
				panic(err)
			}

			filename := fmt.Sprintf("%s %s.cbz", title, chapter.Title)
			fmt.Printf("- %s %s\n", color.GreenString("saving file"), color.BlackString(filename))
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
	cc.Init(&cc.Config{
		RootCmd:       rootCmd,
		Headings:      cc.HiCyan + cc.Bold,
		Commands:      cc.HiYellow + cc.Bold,
		Aliases:       cc.Bold + cc.Italic,
		CmdShortDescr: cc.HiRed,
		ExecName:      cc.Bold,
		Flags:         cc.Bold,
		FlagsDescr:    cc.HiMagenta,
		FlagsDataType: cc.Italic,
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// init sets the flags for the root command
func init() {
	rootCmd.Flags().StringP("language", "l", "", "only download the specified language")
}
