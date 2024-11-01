package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/voxelost/manga-downloader/downloader"
	"github.com/voxelost/manga-downloader/grabber"
	"github.com/voxelost/manga-downloader/packer"
	"github.com/voxelost/manga-downloader/ranges"
)

func Hander(cmd *cobra.Command, args []string) {
	s, errs := grabber.NewSite(getURLArg(args), &settings)
	if len(errs) > 0 {
		var errorStrings []string
		for _, err := range errs {
			errorStrings = append(errorStrings, err.Error())
		}

		slog.Error(fmt.Sprintf("errors testing site (a site may be down): %s", strings.Join(errorStrings, ", ")))
		os.Exit(1)
	}
	if s == nil {
		slog.Error("site not recognised")
		os.Exit(1)
	}
	s.InitFlags(cmd)

	// fetch series title
	title, err := s.FetchTitle()
	if err != nil {
		slog.Error(fmt.Sprintf("error fetching title %q: %v", title, err))
		os.Exit(1)
	}

	// fetch all chapters
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 {
		var errorStrings []string
		for _, err := range errs {
			errorStrings = append(errorStrings, err.Error())
		}

		slog.Error(fmt.Sprintf("errors fetching chapters: %s", strings.Join(errorStrings, ", ")))
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
			slog.Info("canceled by user")
			os.Exit(0)
		}

		rngs = []ranges.Range{{Start: 1, End: int64(lastChapter)}}
	} else {
		// ranges parsing
		settings.Range = getRangesArg(args)
		rngs = ranges.Parse(settings.Range)
	}

	// sort and filter specified ranges
	chapters = chapters.FilterRanges(rngs)

	if len(chapters) == 0 {
		slog.Error("no chapters found for the specified ranges")
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
				slog.Error(fmt.Sprintf("error fetching chapter %q: %v", chap.GetTitle(), err))
				<-g
				return
			}
			slog.Info(fmt.Sprintf("fetched %q - %q", title, chapter.GetTitle()))

			files, err := downloader.FetchChapter(s, chapter)
			if err != nil {
				slog.Error(fmt.Sprintf("error downloading chapter %q: %v", chapter.GetTitle(), err))
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
					slog.Info(fmt.Sprintf("saved file %q", filename))
				} else {
					slog.Error(err.Error())
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
		slog.Error(err.Error())
		os.Exit(1)
	}

	slog.Info(fmt.Sprintf("saved file %q", filename))
}
