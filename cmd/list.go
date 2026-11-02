// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// listJSON prints the listing as JSON instead of a table
var listJSON bool

// listCmd prints what a series offers without downloading anything (#436).
// Intentionally CLI-only: the app shows the same in its chapter panel.
var listCmd = &cobra.Command{
	Use:   "list [flags] url",
	Short: "Lists a series' chapters, languages, groups and volumes without downloading",
	Long: `Lists what a series URL offers without downloading anything: its chapters (number, title, language, group, volume and whether they're locked), the languages and groups --language and --scanlator take, and the site's volumes as a --volumes value.

--language and --scanlator filter the listing as they filter a download, so it shows what the download would take. --json prints it as JSON on stdout, with messages and errors on stderr.`,
	Example: colorizeHelp(`  manga-downloader list https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover

Would list the chapters of Black Clover on mangadex.org.

  manga-downloader list --language es --json https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover

Would print the Spanish chapters of Black Clover as JSON.`),
	Args: cobra.ExactArgs(1),
	Run:  runList,
}

// listing is what list prints, and the shape of its JSON
type listing struct {
	Title string `json:"title"`
	// Grabber is the grabber that matched the URL
	Grabber  string          `json:"grabber"`
	Chapters []listedChapter `json:"chapters"`
	// Languages and Scanlators are the values --language and --scanlator take,
	// null for a filter the site doesn't list
	Languages  *listedFilter `json:"languages"`
	Scanlators *listedFilter `json:"scanlators"`
	// Volumes is the site's volumes as a --volumes value, "" when it has none
	// or they can't be written so
	Volumes string `json:"volumes"`
}

// listedChapter is a release of the listing
type listedChapter struct {
	// ID is the release's ID on the site, left out when the grabber sets none
	ID     string  `json:"id,omitempty"`
	Number float64 `json:"number"`
	// Unnumbered is set when the site lists the chapter without a number
	Unnumbered bool   `json:"unnumbered,omitempty"`
	Title      string `json:"title"`
	Language   string `json:"language,omitempty"`
	// Group is the display string of the release's groups
	Group  string `json:"group,omitempty"`
	Volume int    `json:"volume,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

// listedFilter is what a filter takes: the values, and whether the site also
// takes "all"
type listedFilter struct {
	Values []string `json:"values"`
	All    bool     `json:"all"`
	// Current is the value listed when no filter is set, for sites that list
	// one edition at a time
	Current string `json:"current,omitempty"`
}

func runList(cmd *cobra.Command, args []string) {
	if listJSON {
		// stdout carries the JSON alone
		color.Output = color.Error
	}

	defer browser.Close()
	stopSignals := closeBrowserOnInterrupt()
	defer stopSignals()
	notice.SetHandler(printNotice)
	defer notice.SetHandler(nil)
	browser.SetAttentionHandler(printAttention)
	defer browser.SetAttentionHandler(nil)
	browser.SetVisible(settings.BrowserVisible)

	l := listingOf(openSeries(args[0]))
	if listJSON {
		cerr(writeListingJSON(os.Stdout, l), "Error writing the listing: ")
		return
	}
	cerr(writeListingTable(color.Output, l), "Error writing the listing: ")
}

// listingOf turns an opened series into its listing
func listingOf(s *core.Series) listing {
	l := listing{
		Title:      s.Title,
		Grabber:    grabberName(s.Site),
		Chapters:   make([]listedChapter, 0, len(s.Chapters)),
		Languages:  filterOf(s.Languages),
		Scanlators: filterOf(s.Scanlators),
		Volumes:    core.SiteVolumes(s.Chapters),
	}
	for _, ch := range s.Chapters {
		row := listedChapter{
			Number:     ch.GetNumber(),
			Unnumbered: grabber.Unnumbered(ch),
			Title:      grabber.ChapterLabel(ch),
			Volume:     grabber.VolumeOf(ch),
			Locked:     grabber.LockedOf(ch),
		}
		if c, ok := ch.(interface{ GetID() string }); ok {
			row.ID = c.GetID()
		}
		if c, ok := ch.(interface{ GetLanguage() string }); ok {
			row.Language = c.GetLanguage()
		}
		if c, ok := ch.(interface{ GetGroup() string }); ok {
			row.Group = c.GetGroup()
		}
		l.Chapters = append(l.Chapters, row)
	}
	return l
}

// grabberName is the name of the grabber type behind site ("Mangadex")
func grabberName(site grabber.Site) string {
	if site == nil {
		return ""
	}
	name := fmt.Sprintf("%T", site)
	return name[strings.LastIndex(name, ".")+1:]
}

func filterOf(o *grabber.FilterOptions) *listedFilter {
	if o == nil {
		return nil
	}
	sorted := o.Sorted()
	values := sorted.Values
	if values == nil {
		values = []string{}
	}
	return &listedFilter{Values: values, All: sorted.All, Current: sorted.Current}
}

func writeListingJSON(w io.Writer, l listing) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(l)
}

// writeListingTable prints the listing as a table, with the language, group,
// volume and locked columns only when some chapter has them
func writeListingTable(w io.Writer, l listing) error {
	var showLang, showGroup, showVolume, showLocked bool
	for _, c := range l.Chapters {
		showLang = showLang || c.Language != ""
		showGroup = showGroup || c.Group != ""
		showVolume = showVolume || c.Volume > 0
		showLocked = showLocked || c.Locked
	}

	fmt.Fprintf(w, "%s %s\n", color.New(color.Bold).Sprint(l.Title), color.HiBlackString("(%s, %d chapters)", l.Grabber, len(l.Chapters)))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := []string{"NUMBER", "TITLE"}
	if showLang {
		header = append(header, "LANGUAGE")
	}
	if showGroup {
		header = append(header, "GROUP")
	}
	if showVolume {
		header = append(header, "VOLUME")
	}
	if showLocked {
		header = append(header, "LOCKED")
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, c := range l.Chapters {
		number := strconv.FormatFloat(c.Number, 'f', -1, 64)
		if c.Unnumbered {
			number = "-"
		}
		row := []string{number, truncateString(c.Title, 60)}
		if showLang {
			row = append(row, c.Language)
		}
		if showGroup {
			row = append(row, c.Group)
		}
		if showVolume {
			volume := ""
			if c.Volume > 0 {
				volume = strconv.Itoa(c.Volume)
			}
			row = append(row, volume)
		}
		if showLocked {
			locked := ""
			if c.Locked {
				locked = "yes"
			}
			row = append(row, locked)
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if l.Languages != nil {
		fmt.Fprintf(w, "Languages (--language): %s\n", filterText(l.Languages))
	}
	if l.Scanlators != nil && len(l.Scanlators.Values) > 0 {
		fmt.Fprintf(w, "Groups (--scanlator): %s\n", filterText(l.Scanlators))
	}
	if l.Volumes != "" {
		fmt.Fprintf(w, "Volumes (--volumes): %s\n", l.Volumes)
	}
	return nil
}

// filterText writes a filter's values, then "all" when the site takes it
func filterText(f *listedFilter) string {
	values := f.Values
	if f.All {
		values = append(values[:len(values):len(values)], "all")
	}
	return strings.Join(values, ", ")
}

func init() {
	listCmd.Flags().StringVarP(&settings.Language, "language", "l", "", `only list the specified language ("all" lists every language, on sites listing several)`)
	listCmd.Flags().StringVarP(&settings.Scanlator, "scanlator", "s", "", `only list the releases of the specified scanlation group (the default, and "all", list every group's)`)
	listCmd.Flags().BoolVar(&settings.BrowserVisible, "browser-visible", false, "always show the browser window; rarely needed, as it opens on its own for sites that need it")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "print the listing as JSON on stdout, with messages and errors on stderr")
	rootCmd.AddCommand(listCmd)
}
