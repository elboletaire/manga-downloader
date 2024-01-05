/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	cc "github.com/ivanpirog/coloredcobra"
)

var settings grabber.Settings

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "manga-downloader [flags] [url] [ranges]",
	Short: "Helps you download mangas from websites to CBZ files",

	Long: `With manga-downloader you can easily convert/download web based mangas to CBZ files.

You only need to specify the URL of the manga and the chapters you want to download as a range.

Note the URL must be of the index of the manga, not a single chapter.`,
	Example: colorizeHelp(`  manga-downloader https://inmanga.com/ver/manga/Fire-Punch/17748683-8986-4628-934a-e94a47fe5d59

Would ask you if you want to download all chapters of Fire Punch (1-83).

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10

Would download chapters 1 to 10 of Dr. Stone from inmanga.com.

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10,12,15-20

Would download chapters 1 to 10, 12 and 15 to 20 of Dr. Stone from inmanga.com.

  manga-downloader --language es https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

Would download chapters 10 to 20 of Black Clover from mangadex.org in Spanish.

  manga-downloader --language es --bundle https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

It would also download chapters 10 to 20 of Black Clover from mangadex.org in Spanish, but in this case would bundle them into a single file.

Note arguments aren't really positional, you can specify them in any order:

  manga-downloader --language es 10-20 https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover --bundle`),
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s, errs := grabber.NewSite(getUrlArg(args), &settings)
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

		chapters = chapters.SortByNumber()

		var rngs []ranges.Range
		// ranges argument is not provided
		if len(args) == 1 {
			lastChapter := chapters[len(chapters)-1].GetNumber()
			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Do you want to download all %g chapters", lastChapter),
				IsConfirm: true,
			}

			_, err := prompt.Run()

			if err != nil {
				color.Yellow("Canceled by user")
				os.Exit(0)
			}

			rngs = []ranges.Range{{Begin: 1, End: int64(lastChapter)}}
		} else {
			// ranges parsing
			settings.Range = getRangesArg(args)
			rngs, err = ranges.Parse(settings.Range)
			cerr(err, "Error parsing ranges: %s")
		}

		// sort and filter specified ranges
		chapters = chapters.FilterRanges(rngs)

		if len(chapters) == 0 {
			color.Yellow("No chapters found for the specified ranges")
			os.Exit(1)
		}

		// download chapters
		wg := sync.WaitGroup{}
		g := make(chan struct{}, s.GetMaxConcurrency().Chapters)
		downloaded := grabber.Filterables{}

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

				d := &packer.DownloadedChapter{
					Chapter: chapter,
					Files:   files,
				}

				if !settings.Bundle {
					filename, err := packer.PackSingle(settings.OutputDir, s, d)
					if err == nil {
						fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(filename))
					} else {
						color.Red(err.Error())
					}
				} else {
					// avoid adding it to memory if we're not gonna use it
					downloaded = append(downloaded, d)
				}

				// release guard
				<-g
			}(chap)
		}
		wg.Wait()
		close(g)

		if !settings.Bundle {
			// if we're not bundling, just finish it
			os.Exit(0)
		}

		// resort downloaded
		downloaded = downloaded.SortByNumber()

		dc := []*packer.DownloadedChapter{}
		// convert slice back to DownloadedChapter
		for _, d := range downloaded {
			dc = append(dc, d.(*packer.DownloadedChapter))
		}

		filename, err := packer.PackBundle(settings.OutputDir, s, dc, settings.Range)
		if err != nil {
			color.Red(err.Error())
			os.Exit(1)
		}

		fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(filename))
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
	rootCmd.Flags().BoolVarP(&settings.Bundle, "bundle", "b", false, "bundle all specified chapters into a single file")
	rootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Chapters, "concurrency", "c", 5, "number of concurrent chapter downloads, hard-limited to 5")
	rootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Pages, "concurrency-pages", "C", 10, "number of concurrent page downloads, hard-limited to 10")
	rootCmd.Flags().StringVarP(&settings.Language, "language", "l", "", "only download the specified language")
	rootCmd.Flags().StringVarP(&settings.FilenameTemplate, "filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename")
	// set as persistent, so version command does not complain about the -o flag set via docker
	rootCmd.PersistentFlags().StringVarP(&settings.OutputDir, "output-dir", "o", "./", "output directory for the downloaded files")
}

func cerr(err error, prefix string) {
	if err != nil {
		fmt.Println(color.RedString(prefix + err.Error()))
		os.Exit(1)
	}
}

func colorizeHelp(help string) string {
	// command in yellow
	yre := regexp.MustCompile(`manga-downloader|nada`)
	help = yre.ReplaceAllStringFunc(help, func(s string) string {
		return color.YellowString(s)
	})

	// arguments in gray
	gre := regexp.MustCompile(`http[^ ]*|[\d]+-[\d,-]+`)
	help = gre.ReplaceAllStringFunc(help, func(s string) string {
		return color.HiBlackString(s)
	})

	// --arguments in blue
	bre := regexp.MustCompile(`((--language|--bundle)( es)?)`)
	help = bre.ReplaceAllStringFunc(help, func(s string) string {
		return color.HiBlueString(s)
	})

	return help
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

func getUrlArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	if strings.HasPrefix(args[0], "http") {
		return args[0]
	}

	return args[1]
}
