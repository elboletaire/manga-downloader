package downloader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/elboletaire/manga-downloader/models"
	"github.com/fatih/color"
)

type File struct {
	Data []byte
	Name string
}

type Files []*File

func FetchChapter(chapter models.Chapter) (files Files, err error) {
	for _, page := range chapter.Pages {
		filename := fmt.Sprintf("%03d.jpg", page.Number)
		color.Blue("- downloading %s\n", filename)
		file, err := FetchFile(page.URL, filename)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return
}

// FetchFiles gets an online file returning a new *File
func FetchFile(URL, filename string) (file *File, err error) {
	body, err := Get(URL)
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

// Get is a helper method for obtaining online files via GET call
func Get(URL string) (body io.ReadCloser, err error) {
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(URL)
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
