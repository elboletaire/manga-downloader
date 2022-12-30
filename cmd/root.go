/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/html"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/elgs/gojq"
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
		// uuid match regex
		re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
		id := re.FindString(args[0])

		// retrieve title
		rbody, err := downloader.Get(args[0])
		if err != nil {
			panic(err)
		}
		defer rbody.Close()
		body, err := ioutil.ReadAll(rbody)
		if err != nil {
			panic(err)
		}
		doc := html.Reader(string(body))
		title := html.Query(doc, "h1").FirstChild.Data

		// retrieve chapters json from server
		rbody, err = downloader.Get("https://inmanga.com/chapter/getall?mangaIdentification=" + id)
		if err != nil {
			panic(err)
		}
		defer rbody.Close()
		body, err = ioutil.ReadAll(rbody)
		if err != nil {
			panic(err)
		}
		parser, err := gojq.NewStringQuery(string(body))
		if err != nil {
			panic(err)
		}
		data, _ := parser.QueryToString("data")
		ps, err := gojq.NewStringQuery(data)
		if err != nil {
			panic(err)
		}
		cps, err := ps.Query("result")
		if err != nil {
			panic(err)
		}
		// create chapters slice
		chapters := grabber.NewSlice(cps.([]interface{})).SortByNumber()
		rngs, err := ranges.Parse(args[1])
		if err != nil {
			panic(err)
		}
		// filter specified chapter ranges
		chapters = chapters.GetRanges(rngs)

		// loop chapters to retrieve pages
		for _, chap := range chapters {
			color.Green("Grabbing info for \"%s %s\" with id %s", title, chap.Title, chap.Identification)
			h, err := downloader.Get("https://inmanga.com/chapter/chapterIndexControls?identification=" + chap.Identification)
			if err != nil {
				panic(err)
			}
			defer h.Close()
			strhtml, err := ioutil.ReadAll(h)
			if err != nil {
				panic(err)
			}

			// fmt.Println(string(strhtml))
			doc := html.Reader(string(strhtml))
			cchap := grabber.Chapter{
				Number:     chap.Number,
				PagesCount: int64(chap.PagesCount),
			}

			s := html.Query(doc, "select.PageListClass")
			for _, opt := range html.QueryAll(s, "option") {
				page, _ := strconv.ParseInt(opt.FirstChild.Data, 10, 64)
				cchap.Pages = append(cchap.Pages, grabber.Page{
					Number: page,
					URL:    "https://pack-yak.intomanga.com/images/manga/MANGA-SERIES/chapter/CHAPTER/page/PAGE/" + opt.Attr[0].Val,
				})
			}

			files, err := downloader.FetchChapter(cchap)
			if err != nil {
				panic(err)
			}

			filename := fmt.Sprintf("%s %s.cbz", title, chap.Title)
			color.Blue("Saving file as %s", filename)
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
}
