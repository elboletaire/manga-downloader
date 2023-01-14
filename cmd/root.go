/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"

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

Would download chapters 1 to 10 of Dr. Stone from
inmanga.com

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10,12,15-20

Would download chapters 1 to 10, 12 and 15 to 20 of
Dr. Stone from inmanga.com

  manga-downloader --language es https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

Would download chapters 10 to 20 of Black Clover from
mangadex.org in Spanish`, "manga-downloader", color.YellowString("manga-downloader")),
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		s, errs := grabber.NewSite(args[0])
		if len(errs) > 0 {
			color.Red("Errors testing site (a site may be down):")
			for _, err := range errs {
				color.Red(err.Error())
			}
		}
		if s == nil {
			color.Yellow("Site not recognised")
			os.Exit(1)
		}
		s.InitFlags(cmd)

		// ranges parsing
		rngs, err := ranges.Parse(args[1])
		cerr(err, "Error parsing ranges: %s")

		// fetch series title
		title, err := s.FetchTitle()
		cerr(err, "Error fetching title: %s")

		// fetch all chapters
		chapters, errs := s.FetchChapters()
		if len(errs) > 0 {
			color.Red("Errors fetching chapters:")
			for _, err := range errs {
				color.Red(err.Error())
			}
			os.Exit(1)
		}

		// sort and filter specified ranges
		chapters = chapters.FilterRanges(rngs)

		if len(chapters) == 0 {
			warn("No chapters found for the specified ranges")
		}

		wg := sync.WaitGroup{}
		g := make(chan struct{}, s.GetMaxConcurrency().Chapters)

		// loop chapters to retrieve pages
		for _, chap := range chapters {
			g <- struct{}{}
			wg.Add(1)
			go func(chap grabber.Filterable) {
				defer wg.Done()
				chapter, err := s.FetchChapter(chap)
				if err != nil {
					color.Red("- error fetching chapter %s: %s", chap.GetTitle(), err.Error())
					<-g
					return
				}
				fmt.Printf("fetched %s %s\n", color.CyanString(title), color.HiBlackString(chapter.GetTitle()))

				files, err := downloader.FetchChapter(s, chapter)
				if err != nil {
					color.Red("- error downloading chapter %s: %s", chapter.GetTitle(), err.Error())
					<-g
					return
				}

				filename, err := packer.NewFilenameFromTemplate(title, chapter, s.GetFilenameTemplate())
				if err != nil {
					color.Red("- error creating filename for chapter %s: %s", chapter.GetTitle(), err.Error())
					<-g
					return
				}

				filename += ".cbz"

				if err = packer.ArchiveCBZ(filename, files); err != nil {
					color.Red("- error saving file %s: %s", filename, err.Error())
				} else {
					fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(filename))
				}

				// release guard
				<-g
			}(chap)
		}
		wg.Wait()
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
	rootCmd.Flags().Uint8P("concurrency", "c", 5, "number of concurrent chapter downloads, hard-limited to 5")
	rootCmd.Flags().Uint8P("concurrency-pages", "C", 10, "number of concurrent page downloads, hard-limited to 10")
	rootCmd.Flags().StringP("language", "l", "", "only download the specified language")
	rootCmd.Flags().StringP("filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename")
}

func cerr(err error, prefix string) {
	if err != nil {
		fmt.Println(color.RedString(prefix + err.Error()))
		os.Exit(1)
	}
}

func warn(err string) {
	fmt.Println(color.YellowString(err))
	os.Exit(1)
}
