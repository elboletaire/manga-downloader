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
	"syscall"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/term"

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
	Run:  Run,
}

// Run is the main function of the root command, the main downloading cmd
func Run(cmd *cobra.Command, args []string) {
	// ensure the shared Chrome process (if any) is killed on exit
	defer browser.Close()
	browser.SetVisible(settings.BrowserVisible)

	if settings.Format != packer.FormatCBZ && settings.Format != packer.FormatRaw {
		color.Red("Error: invalid format %q, must be %q or %q", settings.Format, packer.FormatCBZ, packer.FormatRaw)
		exit(1)
	}

	s, errs := grabber.NewSite(getUrlArg(args), &settings)
	if len(errs) > 0 {
		color.Red("Errors testing site (a site may be down):")
		for _, err := range errs {
			color.Red(err.Error())
		}
	}
	if s == nil {
		color.Yellow("Site not recognised")
		exit(1)
	}
	s.InitFlags(cmd)

	// fetch series title
	title, err := s.FetchTitle()
	cerr(err, "Error fetching title: ")

	// fetch all chapters
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 {
		color.Red("Errors fetching chapters:")
		for _, err := range errs {
			color.Red(err.Error())
		}
		exit(1)
	}

	chapters = chapters.SortByNumber()

	// an empty result at this point usually means the language filter matched
	// nothing (e.g. an invalid language code); without this guard the prompt
	// path below would panic indexing an empty slice
	if len(chapters) == 0 {
		color.Yellow(noChaptersMessage(settings.Language))
		exit(1)
	}

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
			exit(0)
		}

		rngs = []ranges.Range{{Begin: 1, End: float64(lastChapter)}}
	} else {
		// ranges parsing
		settings.Range = getRangesArg(args)
		rngs, err = ranges.Parse(settings.Range)
		cerr(err, "Error parsing ranges: ")
	}

	// sort and filter specified ranges
	chapters = chapters.FilterRanges(rngs)

	if len(chapters) == 0 {
		color.Yellow("No chapters found for the specified ranges")
		exit(1)
	}

	// download chapters
	wg := sync.WaitGroup{}
	g := make(chan struct{}, s.GetMaxConcurrency().Chapters)
	downloaded := grabber.Filterables{}

	// Current phase for bundle mode progress
	var currentPhase string

	// progress bar
	p := mpb.New(
		mpb.WithWidth(40),
		mpb.WithOutput(color.Output),
		mpb.WithAutoRefresh(),
	)

	blue := color.New(color.FgBlue)

	// Get terminal width for title truncation
	termWidth := getTerminalWidth()
	mangaLen, chapterLen := calculateTitleLengths(termWidth)

	// sites like mangadex can return the same chapter multiple times, once per
	// translated language; tag each bar with its language in that case
	showLanguage := hasDuplicateChapterNumbers(chapters)

	// For bundle mode, create a single progress bar
	var bundleBar *mpb.Bar
	if settings.Bundle {
		// Calculate total pages for bundle mode
		totalPages := int64(0)
		for _, chap := range chapters {
			chapter, err := s.FetchChapter(chap)
			if err == nil && chapter != nil {
				totalPages += chapter.PagesCount
			}
		}

		// Total steps = download + archive for all pages
		bundleBar = p.AddBar(totalPages*2,
			mpb.PrependDecorators(
				// Dynamic status showing current chapter
				decor.Any(func(s decor.Statistics) string {
					return blue.Sprintf("%-30s", currentPhase)
				}, decor.WCSyncWidthR),
				decor.CountersNoUnit("%d/%d", decor.WC{C: decor.DextraSpace}),
			),
			mpb.AppendDecorators(
				decor.Percentage(decor.WC{W: 4}), // W: 4 to remove space before %
				// Status at the end to prevent shifting
				decor.Any(func(s decor.Statistics) string {
					if s.Current >= s.Total {
						return blue.Sprintf(" bundling ")
					}
					return blue.Sprintf(" downloading")
				}, decor.WC{W: 10}),
			),
		)
		currentPhase = "Gathering info..."
	}

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

			// generate the filename for the chapter
			filename, err := packer.NewFilenameFromTemplate(s.GetFilenameTemplate(), packer.NewChapterFileTemplateParts(title, chapter))
			if err != nil {
				color.Red("- error creating filename for chapter %s: %s", chapter.GetTitle(), err.Error())
				<-g
				return
			}
			if settings.Format != packer.FormatRaw {
				// raw format writes a folder, not a single file
				filename += ".cbz"
			}

			var bar *mpb.Bar
			if !settings.Bundle {
				// For non-bundle mode, create a single bar per chapter that combines download + archive
				// (using the fetched chapter, whose title includes the chapter number)
				barTitle := chapterBarTitle(title, chapter, mangaLen, chapterLen, showLanguage)
				// Total steps = pages (download) + pages (archive)
				total := chapter.PagesCount * 2
				bar = p.AddBar(total,
					mpb.PrependDecorators(
						decor.Name(barTitle, decor.WCSyncWidthR),
						decor.CountersNoUnit("%d/%d", decor.WC{C: decor.DextraSpace}),
					),
					mpb.AppendDecorators(
						decor.Percentage(decor.WC{W: 4}), // W: 4 to remove space before %
						// Status at the end to prevent shifting
						decor.Any(func(s decor.Statistics) string {
							if s.Current >= total/2 {
								return blue.Sprintf(" archiving ")
							}
							return blue.Sprintf(" downloading") // Add space before status
						}, decor.WC{W: 10}),
					),
				)
			} else {
				bar = bundleBar
				currentPhase = fmt.Sprintf("Downloading %s", chapter.GetTitle())
			}

			files, err := downloader.FetchChapter(s, chapter, func(page int, idx int, err error) {
				if err != nil {
					if !settings.Bundle {
						bar.SetCurrent(int64(idx))
					}
					color.Red("- error downloading page %d: %s", idx+1, err.Error())
				} else {
					// Increment by 1 since we're processing one page at a time
					bar.IncrBy(1)
				}
			})
			if err != nil {
				color.Red("- error downloading chapter %s: %s", chapter.GetTitle(), err.Error())
				if !settings.Bundle {
					bar.Abort(false)
				}
				<-g
				return
			}

			d := &packer.DownloadedChapter{
				Chapter: chapter,
				Files:   files,
			}

			if !settings.Bundle {
				_, err := packer.PackSingle(settings.OutputDir, s, d, func(page, _ int) {
					bar.IncrBy(1) // Increment archive progress
				})
				if err != nil {
					color.Red(err.Error())
				}
				if !bar.Completed() {
					// the bar can no longer reach its total (packing failed or
					// pages were skipped); mark it done so p.Wait() won't hang
					bar.Abort(false)
				}
			} else {
				// For bundle mode, increment archive progress
				bar.IncrBy(int(chapter.PagesCount))
				// avoid adding it to memory if we're not gonna use it
				downloaded = append(downloaded, d)
			}

			// release guard
			<-g
		}(chap)
	}
	// wait for all routines to finish
	wg.Wait()
	close(g)

	if !settings.Bundle {
		// let the render loop paint the final state of the bars before
		// exiting; without this, fast packing (e.g. raw folders) ends with
		// the last painted frame stuck at whatever the previous refresh saw
		p.Wait()
		// if we're not bundling, we're done
		exit(0)
	}

	// resort downloaded
	downloaded = downloaded.SortByNumber()

	dc := []*packer.DownloadedChapter{}
	tp := 0
	// convert slice back to DownloadedChapter
	for _, d := range downloaded {
		chapter := d.(*packer.DownloadedChapter)
		dc = append(dc, chapter)
		tp += int(chapter.PagesCount)
	}

	if settings.Bundle {
		currentPhase = fmt.Sprintf("Creating bundle for chapters %s", settings.Range)
	}

	filename, err := packer.PackBundle(settings.OutputDir, s, dc, settings.Range, func(page, _ int) {
		if bundleBar != nil {
			bundleBar.IncrBy(page)
		}
	})

	if bundleBar != nil && !bundleBar.Completed() {
		// failed chapters leave the bundle bar short of its total; mark it
		// done so p.Wait() won't hang
		bundleBar.Abort(false)
	}
	// flush the final render before printing the outcome
	p.Wait()

	if err != nil {
		color.Red(err.Error())
		exit(1)
	}

	fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(filename))
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
		exit(1)
	}
}

// init sets the flags for the root command
func init() {
	rootCmd.Flags().BoolVarP(&settings.Bundle, "bundle", "b", false, "bundle all specified chapters into a single file")
	rootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Chapters, "concurrency", "c", 5, "number of concurrent chapter downloads, hard-limited to 5")
	rootCmd.Flags().Uint8VarP(&settings.MaxConcurrency.Pages, "concurrency-pages", "C", 10, "number of concurrent page downloads, hard-limited to 10")
	rootCmd.Flags().StringVarP(&settings.Language, "language", "l", "", "only download the specified language")
	rootCmd.Flags().StringVarP(&settings.FilenameTemplate, "filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename")
	rootCmd.Flags().StringVarP(&settings.Format, "format", "f", packer.FormatCBZ, "output format: cbz or raw (a folder with the images)")
	rootCmd.Flags().BoolVar(&settings.BrowserVisible, "browser-visible", false, "open the browser window from the start (it opens automatically anyway when a headless attempt hits a challenge)")
	rootCmd.Flags().Uint8VarP(&settings.Retry, "retry", "r", 1, "number of retries for failed page downloads, hard-limited to 3 (0 disables retrying)")
	// set as persistent, so version command does not complain about the -o flag set via docker
	rootCmd.PersistentFlags().StringVarP(&settings.OutputDir, "output-dir", "o", "./", "output directory for the downloaded files")
}

// exit closes the shared browser (if any was started) before exiting,
// otherwise the Chrome process would be left running in the background
func exit(code int) {
	browser.Close()
	os.Exit(code)
}

func cerr(err error, prefix string) {
	if err != nil {
		fmt.Println(color.RedString(prefix + err.Error()))
		exit(1)
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

// getTerminalWidth returns the current terminal width or a default value if it can't be determined
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(syscall.Stdin))
	if err != nil {
		return 80 // default terminal width
	}
	return width
}

// calculateTitleLengths calculates how much space to allocate for manga and chapter titles
func calculateTitleLengths(termWidth int) (mangaLen, chapterLen int) {
	// Reserve space for other elements in the progress bar:
	// - status (e.g. "downloading", "archiving", etc.): ~15 chars
	// - progress counter (e.g. "123/456"): ~10 chars
	// - percentage: ~5 chars
	// - decorators (spaces, colons, etc.): ~5 chars
	reservedSpace := 35
	availableSpace := termWidth - reservedSpace

	// Allocate 60% to manga title and 40% to chapter title if there's enough space
	if availableSpace > 20 {
		mangaLen = (availableSpace * 60) / 100
		chapterLen = (availableSpace * 40) / 100
	} else {
		// If space is very limited, use minimal lengths
		mangaLen = 10
		chapterLen = 10
	}

	return
}

// truncateString truncates the input string at a specified maximum length
// without cutting words. It finds the last space within the limit and truncates there.
// Length is counted in runes, so multi-byte titles are never split mid-character.
func truncateString(input string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}

	runes := []rune(input)
	if len(runes) <= maxLength {
		return input
	}

	// Find the last space before maxLength
	truncated := string(runes[:maxLength])
	truncationPoint := strings.LastIndex(truncated, " ")
	if truncationPoint == -1 {
		// No spaces found, force to maxLength (cuts the word)
		return truncated + "..."
	}

	// Return substring up to the last found space
	return truncated[:truncationPoint] + "..."
}

// chapterBarTitle builds the label of a chapter's progress bar from the fetched
// chapter data. When showLanguage is set (multiple versions of the same chapter
// are being downloaded, e.g. one per translated language), the chapter language
// is appended after the title so same-numbered bars can be told apart. The tag
// is added after truncation so it never gets cut off.
func chapterBarTitle(seriesTitle string, chapter *grabber.Chapter, mangaLen, chapterLen int, showLanguage bool) string {
	chapTitle := truncateString(chapter.GetTitle(), chapterLen)
	if showLanguage && chapter.Language != "" {
		chapTitle += " [" + chapter.Language + "]"
	}

	return fmt.Sprintf("%s - %s:", truncateString(seriesTitle, mangaLen), chapTitle)
}

// noChaptersMessage builds the error message shown when a fetch returns no
// chapters, pointing at the language filter when one was set
func noChaptersMessage(language string) string {
	if language == "" {
		return "No chapters found"
	}

	return fmt.Sprintf(
		"No chapters found for language %q (perhaps the site uses a different language code, e.g. %q for Latin American Spanish)",
		language, "es-la",
	)
}

// hasDuplicateChapterNumbers reports whether the same chapter number appears
// more than once, which happens when a site returns one entry per translated
// language for the same chapter
func hasDuplicateChapterNumbers(chapters grabber.Filterables) bool {
	seen := map[float64]bool{}
	for _, c := range chapters {
		if seen[c.GetNumber()] {
			return true
		}
		seen[c.GetNumber()] = true
	}

	return false
}

func toMetaFunc(c *color.Color) func(string) string {
	return func(s string) string {
		return c.Sprint(s)
	}
}
