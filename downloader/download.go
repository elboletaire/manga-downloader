package downloader

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"github.com/elboletaire/manga-downloader/grabber"
)

type File struct {
	Data []byte
	Name string
}

type Files []*File

func FetchChapter(chapter grabber.Chapter) (files Files, err error) {
	for _, page := range chapter.Pages {
		file, err := FetchFile(page.URL, fmt.Sprintf("%03d.jpg", page.Number))
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return
}

func FetchFile(URL, filename string) (file *File, err error) {
	body, err := Get(URL)
	if err != nil {
		return
	}

	data, err := ioutil.ReadAll(body)
	if err != nil {
		return
	}

	file = &File{
		Data: data,
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

func DownloadChapter(chapter grabber.Chapter) error {
	for _, page := range chapter.Pages {
		err := DownloadFile(page.URL, fmt.Sprint(int64(chapter.Number)), fmt.Sprintf("%03d.jpg", page.Number))
		if err != nil {
			return err
		}
	}

	return nil
}

func DownloadFile(URL, folder, filename string) error {
	body, err := Get(URL)
	if err != nil {
		return err
	}

	file, err := os.Create(path.Join("/tmp", folder, filename))
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, body)
	if err != nil {
		return err
	}

	return nil
}
