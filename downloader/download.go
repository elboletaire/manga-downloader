package downloader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/elboletaire/manga-downloader/models"
	"github.com/fatih/color"
)

type File struct {
	Data []byte
	Name string
}

type Files []*File

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site models.Site, chapter models.Chapter) (files Files, err error) {
	var wg sync.WaitGroup
	for _, page := range chapter.Pages {
		wg.Add(1)
		go func(page models.Page) {
			defer wg.Done()

			filename := fmt.Sprintf("%03d.jpg", page.Number)
			fmt.Println(color.BlueString("- downloading %s", filename))
			file, err := FetchFile(GetParams{
				URL:     page.URL,
				Referer: site.GetBaseUrl(),
			}, filename)

			if err != nil {
				fmt.Println(color.RedString("- error downloading page %s", filename))
				return
			}
			files = append(files, file)
		}(page)
	}

	wg.Wait()

	return
}

// FetchFiles gets an online file returning a new *File
func FetchFile(params GetParams, filename string) (file *File, err error) {
	body, err := Get(params)
	if err != nil {
		return
	}

	data := new(bytes.Buffer)
	io.Copy(data, body)
	if err != nil {
		return
	}

	file = &File{
		Data: data.Bytes(),
		Name: filename,
	}

	return
}

// GetParams is a struct for passing parameters to the Get method
type GetParams struct {
	URL     string
	Referer string
}

// Get is a helper method for obtaining online files via GET call
func Get(params GetParams) (body io.ReadCloser, err error) {
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	req, _ := http.NewRequest("GET", params.URL, nil)
	if params.Referer != "" {
		req.Header.Add("Referer", params.Referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		err = errors.New("received non 200 response code")
		return
	}

	body = resp.Body
	return
}

func GetText(URL string) (body string, err error) {
	rbody, err := Get(GetParams{URL: URL})
	if err != nil {
		return
	}
	defer rbody.Close()

	buf := new(bytes.Buffer)
	io.Copy(buf, rbody)
	body = buf.String()

	return
}
