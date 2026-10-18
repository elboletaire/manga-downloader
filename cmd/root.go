// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	cc "github.com/ivanpirog/coloredcobra"
)

var settings grabber.Settings

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "manga-downloader [flags] [url] [ranges]",
	Short: "Helps you download mangas from websites to CBZ, CBR or EPUB files",

	Long: `With manga-downloader you can easily convert/download web based mangas to CBZ, CBR or EPUB files.

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
	// Capture the stop func so we can pass it to onInterrupt on Ctrl+C, which
	// deregisters the handler before calling closeBrowser — this lets a second
	// Ctrl+C kill the process immediately instead of re-entering closeBrowser.
	stopSignals := closeBrowserOnInterrupt()
	defer stopSignals()
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

	// load the series (identify and open), then close the browser after
	releaseLd := browser.Hold()
	s, errs := core.Identify(getUrlArg(args), &settings)
	if len(errs) > 0 {
		red("Errors testing site (a site may be down):")
		for _, err := range errs {
			red(err.Error())
		}
	}
	if s == nil {
		releaseLd()
		yellow("Site not recognised")
		exit(1)
	}

	series, err := core.Open(s, &settings)
	releaseLd()
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
		if err := confirmAll(allChaptersLabel(chapters)); err != nil {
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

	// locked chapters can't be read logged out: leave them out instead of failing each
	chapters, lockedNote := skipLocked(chapters)
	if lockedNote != "" {
		yellow("%s", lockedNote)
	}
	if len(chapters) == 0 {
		yellow("Every chapter matched by the specified ranges is locked")
		exit(1)
	}

	showLanguage := core.HasDuplicateChapterNumbers(chapters)
	showGroup := core.HasGroupDuplicates(chapters)

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		// Non-TTY (piped, CI, docker without -t): plain line-based output.
		// No interactive bars; one line per chapter start/done/fail.
		// Non-terminal output defaults to 80-column-equivalent logging.
		releaseDl := browser.Hold()
		defer releaseDl()
		ev := plainEvents{showLang: showLanguage, showGroup: showGroup}
		bundleFile, err := series.Download(context.Background(), chapters, skipPages, settings.Range, ev)
		if !settings.Bundle {
			exit(0)
		}
		if err != nil {
			red(err.Error())
			exit(1)
		}
		fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(bundleFile))
		return
	}

	// TTY mode: Bubble Tea interactive progress bars.
	// Background detection happens inside the model via tea.RequestBackgroundColor.
	// Default to dark until the BackgroundColorMsg arrives.
	model := newProgressModel(series.Title, settings.Range, settings.Bundle, showLanguage, showGroup, true)
	prog := tea.NewProgram(model)

	// evDone is closed when prog.Run() returns so that bubbleEvents methods
	// that call prog.Println can fall back to stderr instead of deadlocking.
	evDone := make(chan struct{})
	ev := &bubbleEvents{prog: prog, done: evDone, showLang: showLanguage, showGroup: showGroup}

	// Override the notice and attention handlers with TUI-aware versions that
	// route through prog.Send (safe after quit) instead of printing directly.
	notice.SetHandler(teaNoticeHandler(ev))
	browser.SetAttentionHandler(teaAttentionHandler(ev))

	type downloadResult struct {
		file string
		err  error
	}
	resultCh := make(chan downloadResult, 1)
	releaseDl := browser.Hold()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Restore the terminal before the stack trace is printed.
				prog.Kill()
				releaseDl()
				panic(r)
			}
		}()
		defer releaseDl()
		file, dlErr := series.Download(context.Background(), chapters, skipPages, settings.Range, ev)
		prog.Send(downloadDoneMsg{err: dlErr})
		resultCh <- downloadResult{file: file, err: dlErr}
	}()

	_, runErr := prog.Run()

	// Signal that the program has stopped; ev.safePrintln will use stderr from now on.
	close(evDone)

	// Restore the plain notice and attention handlers after the bars are gone.
	notice.SetHandler(printNotice)
	browser.SetAttentionHandler(printAttention)

	if errors.Is(runErr, tea.ErrInterrupted) {
		// Ctrl+C: deregister the signal handler first so a second Ctrl+C kills
		// the process immediately while closeBrowser runs, matching master's behaviour.
		onInterrupt(os.Interrupt, stopSignals)
		return // unreachable; onInterrupt calls os.Exit
	}
	if runErr != nil {
		// The TUI failed to start (e.g. can't open TTY). Continue and wait for
		// the download goroutine; ev.safePrintln falls back to stderr.
		fmt.Fprintf(os.Stderr, "warning: TUI error: %v\n", runErr)
	}

	res := <-resultCh

	// raised before anything was fetched: --volumes auto on a site without
	// volumes, or a volume filename template that doesn't work
	var serr *core.SettingError
	if errors.As(res.err, &serr) {
		exitSettingError(res.err)
	}

	if !settings.Bundle {
		exit(0)
	}

	// a saved bundle's row already says done, like the chapters'
	if res.err != nil {
		red(res.err.Error())
		exit(1)
	}
	if runErr != nil {
		// unless the bars never showed: then say it like the non-TTY path
		fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(res.file))
	}
}

// exitSettingError prints a setting that failed validation in the words each
// flag has always been reported with, then exits
func exitSettingError(err error) {
	var se *core.SettingError
	errors.As(err, &se)

	switch se.Setting {
	case "format":
		red("Error: invalid format %q, must be %q, %q, %q or %q", se.Value, packer.FormatCBZ, packer.FormatCBR, packer.FormatRaw, packer.FormatEPUB)
	case "reading-direction":
		red("Error: invalid --reading-direction value %q, must be %q or %q", se.Value, packer.ReadingDirectionRTL, packer.ReadingDirectionLTR)
	case "convert-images":
		red("Error: invalid --convert-images value: %s", se.Err)
	case "volumes":
		red("Error: invalid --volumes value: %s", se.Err)
	case "filename-template":
		// a parts run names its files by the chapter template
		what := "the volume filename template"
		if settings.Volumes == core.VolumesParts {
			what = "the chapter filename template"
		}
		red("Error: %s %q doesn't work: %s", what, se.Value, se.Err)
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
	rootCmd.Flags().StringVarP(&settings.Scanlator, "scanlator", "s", "", `only download the releases of the specified scanlation group, for sites hosting several groups' versions of the same chapters (the default, and "all", keep every group's)`)
	rootCmd.Flags().StringVarP(&settings.FilenameTemplate, "filename-template", "t", packer.FilenameTemplateDefault, "template for the resulting filename: {{.Series}}, {{.Number}}, {{.Title}}, {{.SiteTitle}}, {{.Group}}, {{.Volume}} and {{.Version}}; {{pad .Number 4}} zero-pads a number")
	rootCmd.Flags().StringVarP(&settings.Format, "format", "f", packer.FormatCBZ, "output format: cbz, cbr, raw (a folder with the images) or epub")
	rootCmd.Flags().StringVar(&settings.ReadingDirection, "reading-direction", packer.ReadingDirectionRTL, `direction pages turn in epub files: "rtl" for manga, "ltr" for western comics`)
	rootCmd.Flags().StringVar(&settings.ConvertImages, "convert-images", grabber.ConvertImagesDefault, `comma-separated source image formats to convert to jpeg for e-reader compatibility: "avif", "webp" or "none"`)
	rootCmd.Flags().StringVar(&settings.Volumes, "volumes", "", `pack one file per volume: "auto" uses the site's volumes, "parts" packs the parts of a chapter into one file named after it, or give chapter ranges, one group per volume ("1-7/8-16"), "N:" giving a group's number ("16:112-114"); chapters outside every volume are packed on their own`)
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

func toMetaFunc(c *color.Color) func(string) string {
	return func(s string) string {
		return c.Sprint(s)
	}
}

// allChaptersLabel is the prompt for downloading every listed chapter, which
// says how many of them the listing marks as locked
func allChaptersLabel(chapters grabber.Filterables) string {
	label := fmt.Sprintf("Do you want to download all %d chapters", len(chapters))
	locked := 0
	for _, c := range chapters {
		if grabber.LockedOf(c) {
			locked++
		}
	}
	if locked > 0 {
		label += fmt.Sprintf(" (%d locked, skipped)", locked)
	}
	return label
}

// errDeclined is what confirmAll returns for anything but a "y" answer
var errDeclined = errors.New("declined")

// confirmAll asks label as a yes/no question and fails unless the answer is "y".
// It reads the keys itself: a second goroutine redrawing on Enter duplicates the line.
func confirmAll(label string) error {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		if state, err := term.MakeRaw(fd); err == nil {
			defer term.Restore(fd, state)
		}
	}
	return readConfirm(os.Stdin, color.Output, label)
}

// readConfirm draws the question, echoes the answer as it is typed and returns
// nil only for "y" then Enter; anything else declines
func readConfirm(in io.Reader, out io.Writer, label string) error {
	fmt.Fprintf(out, "%s %s %s ",
		color.New(color.FgBlue, color.Bold).Sprint("?"),
		color.New(color.Bold).Sprint(label+"?"),
		color.New(color.Faint).Sprint("[y/N]"))

	var answer []byte
	key := make([]byte, 1)
	for {
		if _, err := in.Read(key); err != nil {
			fmt.Fprint(out, "\r\n")
			return errDeclined
		}
		c := key[0]
		if c == '\r' || c == '\n' {
			break
		}
		switch {
		case c == 3 || c == 4: // Ctrl+C, Ctrl+D
			fmt.Fprint(out, "\r\n")
			return errDeclined
		case c == 27: // an escape sequence (arrow keys...) is not part of the answer
			if err := skipEscape(in, key); err != nil {
				fmt.Fprint(out, "\r\n")
				return errDeclined
			}
		case (c == 127 || c == 8) && len(answer) > 0:
			answer = answer[:len(answer)-1]
			fmt.Fprint(out, "\b \b")
		case c >= ' ' && c < 127:
			answer = append(answer, c)
			_, _ = out.Write(key)
		}
	}

	// the answered line replaces the question
	fmt.Fprintf(out, "\r\x1b[2K%s %s\r\n", color.New(color.Faint).Sprint(label+":"), answer)
	if !strings.EqualFold(string(answer), "y") {
		return errDeclined
	}
	return nil
}

// skipEscape consumes the rest of an escape sequence after its ESC, reading
// through key: a CSI or SS3 sequence runs up to its final byte (0x40-0x7E)
func skipEscape(in io.Reader, key []byte) error {
	if _, err := in.Read(key); err != nil {
		return err
	}
	if key[0] != '[' && key[0] != 'O' {
		return nil
	}
	for {
		if _, err := in.Read(key); err != nil {
			return err
		}
		if key[0] >= 0x40 && key[0] <= 0x7E {
			return nil
		}
	}
}

// skipLocked drops the chapters the listing marks as locked, returning the rest
// and the line that says which were left out ("" when none were)
func skipLocked(chapters grabber.Filterables) (grabber.Filterables, string) {
	open, locked := core.SplitLocked(chapters)
	if len(locked) == 0 {
		return chapters, ""
	}
	return open, core.LockedMessage(locked)
}
