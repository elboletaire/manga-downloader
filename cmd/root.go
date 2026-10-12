// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
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

You can use either a series/index URL or a chapter URL; chapter URLs are automatically resolved to the series.`,
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

  manga-downloader --volumes auto https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 1-30

Would pack chapters 1 to 30 into one file per volume, as the site groups them. Volumes can also be given as chapter ranges, one per volume: --volumes 1-7/8-16.

Note arguments aren't really positional, you can specify them in any order:

  manga-downloader --language es 10-20 https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover --bundle`),
	Args: func(cmd *cobra.Command, args []string) error {
		if v, _ := cmd.Flags().GetBool("version"); v {
			return nil
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	Run: Run,
}

// Run is the main function of the root command, the main downloading cmd
func Run(cmd *cobra.Command, args []string) {
	// `--version` is what the bug-report template asks for; honor it as an
	// alias of the `version` command so both produce identical output
	if v, _ := cmd.Flags().GetBool("version"); v {
		showVersion()
		return
	}

	// ensure the shared Chrome process (if any) is killed on exit, including
	// an interrupted one
	defer browser.Close()
	defer closeBrowserOnInterrupt()()
	notice.SetHandler(printNotice)
	defer notice.SetHandler(nil)
	browser.SetAttentionHandler(printAttention)
	defer browser.SetAttentionHandler(nil)
	browser.SetVisible(settings.BrowserVisible)

	// validated before anything is fetched, so a typo fails fast
	skipPages, err := core.Validate(&settings)
	if err != nil {
		exitSettingError(err)
	}

	s, errs := core.Identify(getUrlArg(args), &settings)
	if len(errs) > 0 {
		red("Errors testing site (a site may be down):")
		for _, err := range errs {
			red(err.Error())
		}
	}
	if s == nil {
		yellow("Site not recognised")
		exit(1)
	}

	series, err := core.Open(s, &settings)
	if err != nil {
		exitOpenError(err)
	}
	chapters := series.Chapters

	var rngs []ranges.Range
	// true only if the user typed a range; distinguishes it from the
	// synthesised "everything" range below
	rangeRequested := len(args) > 1
	// ranges argument is not provided
	if len(args) == 1 {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("Do you want to download all %d chapters", len(chapters)),
			IsConfirm: true,
		}

		_, err := prompt.Run()

		if err != nil {
			yellow("Canceled by user")
			exit(0)
		}

		rngs = []ranges.Range{core.AllChaptersRange(chapters)}
	} else {
		// ranges parsing
		settings.Range = getRangesArg(args)
		rngs, err = ranges.Parse(settings.Range)
		cerr(err, "Error parsing ranges: ")
	}

	// some sites prune old chapters (they soft-404 to the series page), so a
	// user-typed range below the first listed chapter would silently skip
	// that part; warn only when the range was explicit, not synthesised
	if floor := chapters[0].GetNumber(); rangeRequested && core.RequestsBelowListedFloor(rngs, floor) {
		yellow(
			"Only chapters %g and up are listed for this series; the requested chapters below that will be skipped",
			floor,
		)
	}

	// sort and filter specified ranges
	chapters = chapters.FilterRanges(rngs)

	if len(chapters) == 0 {
		yellow("No chapters found for the specified ranges")
		exit(1)
	}

	p := mpb.New(
		// No WithWidth: on a terminal mpb reads the real size each render and
		// the bar filler takes whatever space is left after the decorators.
		// On non-terminal output (piped/redirected) mpb defaults to 80 columns.
		mpb.WithOutput(color.Output),
		mpb.WithAutoRefresh(),
	)

	// Get terminal width for title truncation
	termWidth := getTerminalWidth()
	mangaLen, chapterLen := calculateTitleLengths(termWidth)

	ev := newProgress(p, progressConfig{
		title:  series.Title,
		bundle: settings.Bundle,
		// the range the bundle is named after, shown while it's packed
		bundleRange: settings.Range,
		mangaLen:    mangaLen,
		chapterLen:  chapterLen,
		// sites like mangadex can return the same chapter multiple times, once per
		// translated language; tag each bar with its language in that case
		showLanguage: core.HasDuplicateChapterNumbers(chapters),
	})

	bundleFile, err := series.Download(context.Background(), chapters, skipPages, settings.Range, ev)

	// raised before anything was fetched: --volumes auto on a site without
	// volumes, or a volume filename template that doesn't work
	var serr *core.SettingError
	if errors.As(err, &serr) {
		p.Wait()
		exitSettingError(err)
	}

	if !settings.Bundle {
		// let the render loop paint the final state of the bars before
		// exiting; without this, fast packing (e.g. raw folders) ends with
		// the last painted frame stuck at whatever the previous refresh saw
		p.Wait()
		// if we're not bundling, we're done
		exit(0)
	}

	// failed chapters leave the bundle bar short of its total; mark it
	// done so p.Wait() won't hang
	ev.finishBundle()
	// flush the final render before printing the outcome
	p.Wait()

	if err != nil {
		red(err.Error())
		exit(1)
	}

	fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(bundleFile))
}

// exitSettingError prints a setting that failed validation in the words each
// flag has always been reported with, then exits
func exitSettingError(err error) {
	var se *core.SettingError
	errors.As(err, &se)

	switch se.Setting {
	case "format":
		red("Error: invalid format %q, must be %q or %q", se.Value, packer.FormatCBZ, packer.FormatRaw)
	case "convert-images":
		red("Error: invalid --convert-images value: %s", se.Err)
	case "volumes":
		red("Error: invalid --volumes value: %s", se.Err)
	case "filename-template":
		red("Error: the volume filename template %q doesn't work: %s", se.Value, se.Err)
	default:
		cerr(se.Err, "Error parsing --"+se.Setting+": ")
	}
	exit(1)
}

// exitOpenError prints why the series couldn't be opened, then exits
func exitOpenError(err error) {
	var terr *core.TitleError
	var cerrs *core.ChaptersError
	var nerr *core.NoChaptersError

	switch {
	case errors.As(err, &terr):
		cerr(terr.Err, "Error fetching title: ")
	case errors.As(err, &cerrs):
		red("Errors fetching chapters:")
		for _, err := range cerrs.Errs {
			red(err.Error())
		}
		exit(1)
	case errors.As(err, &nerr):
		// an empty result usually means the language filter matched nothing
		yellow(nerr.Error())
	default:
		red(err.Error())
	}
	exit(1)
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

	// before cobra, which would reject the bare run for missing its URL argument
	if shouldOpenFromExplorer(os.Args[1:], launchedFromExplorer()) {
		openFromExplorer()
		return
	}

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
	rootCmd.Flags().StringVarP(&settings.Language, "language", "l", "", `only download the specified language ("all" downloads every language, on sites listing several)`)
	rootCmd.Flags().StringVarP(&settings.Scanlator, "scanlator", "s", "", `only download the specified scanlation group, for sites hosting several versions of the same chapters ("all" downloads every group's)`)
	rootCmd.Flags().StringVarP(&settings.FilenameTemplate, "filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename")
	rootCmd.Flags().StringVarP(&settings.Format, "format", "f", packer.FormatCBZ, "output format: cbz or raw (a folder with the images)")
	rootCmd.Flags().StringVar(&settings.ConvertImages, "convert-images", grabber.ConvertImagesDefault, `comma-separated source image formats to convert to jpeg for e-reader compatibility: "avif", "webp" or "none"`)
	rootCmd.Flags().StringVar(&settings.Volumes, "volumes", "", `pack one file per volume: "auto" uses the site's volumes, or give chapter ranges, one group per volume ("1-7/8-16"), "N:" giving a group's number ("16:112-114"); chapters outside every volume are packed on their own`)
	rootCmd.Flags().BoolVar(&settings.BrowserVisible, "browser-visible", false, "always show the browser window; rarely needed, as it opens on its own for sites that need it")
	rootCmd.Flags().Uint8VarP(&settings.Retry, "retry", "r", 1, "number of retries for failed page downloads, hard-limited to 3 (0 disables retrying)")
	// no shorthand on purpose: a long flag takes the next argument verbatim as
	// its value (pflag never looks for a leading "-" there), so `--skip-pages
	// -1` works just like `--skip-pages=-1`. A shorthand wouldn't: "-S-1"
	// would have to be spelled that way, with no space
	rootCmd.Flags().StringVar(&settings.SkipPages, "skip-pages", "", `pages to exclude from every chapter, as a range ("2", "2,5-7"); negative positions count from the end of each chapter, so "-1" is its last page`)
	rootCmd.Flags().BoolP("version", "v", false, "show version information (alias of the version command)")
	// set as persistent, so version command does not complain about the -o flag set via docker
	rootCmd.PersistentFlags().StringVarP(&settings.OutputDir, "output-dir", "o", "./", "output directory for the downloaded files")
}

// exit closes the shared browser (if any was started) before exiting,
// otherwise the Chrome process would be left running in the background
func exit(code int) {
	// an interrupted run ends with the signal's code, whatever failed after
	if c := interrupted.Load(); c != 0 {
		code = int(c)
	}
	closeBrowser()
	osExit(code)
}

func cerr(err error, prefix string) {
	if err != nil {
		red("%s", prefix+err.Error())
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
	bre := regexp.MustCompile(`((--language|--bundle|--volumes)( es| auto)?)`)
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

// fixedDecorWidth is the approximate total width consumed by the non-title
// decorators on every bar row:
//   - counter  "9999/9999 " ≈ 10
//   - percentage " 100%"     ≈  5
//   - status   " downloading" ≈ 12
//   - two spaces mpb inserts around the bar filler  = 2
const fixedDecorWidth = 29

// minBarWidth is the minimum filler width we want to leave for the bar.
// The title is pre-capped so that on most terminal sizes the bar is at least
// this wide; on narrower terminals mpb shrinks and eventually truncates the
// title itself rather than letting the bar disappear entirely.
const minBarWidth = 15

// titleSeparatorWidth is the number of characters added around the two title
// parts in the formatted label "manga - chapter:" (space, dash, space, colon).
const titleSeparatorWidth = 4

// calculateTitleLengths returns how many runes the manga-title and
// chapter-title portions of a bar label may each occupy before being
// truncated.  The remaining terminal width (after the full title row,
// the fixed decorators, and the minimum bar) is given to the bar filler.
//
// On terminals that are wide enough the invariant
//
//	termWidth - (mangaLen + titleSeparatorWidth + chapterLen) - fixedDecorWidth >= minBarWidth
//
// holds exactly.  On very narrow terminals (below about 64 columns) the
// parts are set to 1 each so the bar still has some room; mpb will then
// truncate or elide the title further if required.
func calculateTitleLengths(termWidth int) (mangaLen, chapterLen int) {
	// Available runes for mangaLen + chapterLen combined.  The rendered title
	// also uses titleSeparatorWidth characters (" - " and ":") so we subtract
	// that too, leaving at least minBarWidth columns for the filler.
	maxParts := termWidth - fixedDecorWidth - minBarWidth - titleSeparatorWidth
	if maxParts > 60 {
		// Cap on very wide terminals: a 60-rune parts budget is already generous.
		maxParts = 60
	}
	// Always show at least 1 rune per part; mpb will truncate further if needed.
	mangaLen = max(1, maxParts*6/10)
	chapterLen = max(1, maxParts*4/10)
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

func toMetaFunc(c *color.Color) func(string) string {
	return func(s string) string {
		return c.Sprint(s)
	}
}
