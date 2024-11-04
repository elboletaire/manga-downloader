package downloader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"

	"github.com/voxelost/manga-downloader/grabber"
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter) (files []*File, err error) {
	wg := sync.WaitGroup{}

	slog.Debug(fmt.Sprintf("downloading pages for %q", chapter.GetTitle()))
	// guard := make(chan struct{}, site.GetMaxConcurrency().Pages)

	for _, page := range chapter.Pages {
		// guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page) {
			defer wg.Done()

			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, page.URL, http.NoBody)
			if err != nil {
				slog.Error(fmt.Sprintf("error downloading page %d of %q", page.Number, chapter.GetTitle()))
				return
			}

			request.Header.Add("Referer", site.BaseURL())

			resp, err := http.DefaultClient.Do(request)
			if err != nil {
				slog.Error(fmt.Sprintf("error downloading page %d of %q", page.Number, chapter.GetTitle()))
				return
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				slog.Error(fmt.Sprintf("error downloading page %d of %q", page.Number, chapter.GetTitle()))
				return
			}

			resp.Body.Close()

			files = append(files, &File{
				Data: data,
				Page: uint(page.Number),
			})

			// // release guard
			// <-guard
		}(page)
	}
	wg.Wait()
	// close(guard)

	// sort files by page number
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Page < files[j].Page
	})

	return files, nil
}
