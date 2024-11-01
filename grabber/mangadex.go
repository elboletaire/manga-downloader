package grabber

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"

	"github.com/google/uuid"
)

// Mangadex is a grabber for mangadex.org
type Mangadex struct {
	*Grabber
	title string
}

// MangadexChapter represents a MangaDex Chapter
type MangadexChapter struct {
	Chapter
	ID string
}

// ValidateURL checks if the site is MangaDex
func (m *Mangadex) ValidateURL() (bool, error) {
	re := regexp.MustCompile(`mangadex\.org`)
	return re.MatchString(m.URL), nil
}

type mangadexMangaSearchAPIResponse struct {
	Result   string `json:"result"`
	Response string `json:"response"`
	Data     struct {
		ID         uuid.UUID `json:"id"`
		Type       string    `json:"type"`
		Attributes struct {
			Title                  map[string]string   `json:"title"`
			AltTitles              []map[string]string `json:"altTitles"`
			Description            map[string]string   `json:"description"`
			IsLocked               bool                `json:"isLocked"`
			Links                  map[string]string   `json:"links"`
			OriginalLanguage       string              `json:"originalLanguage"`
			LastVolume             string              `json:"lastVolume"`
			LastChapter            string              `json:"lastChapter"`
			PublicationDemographic string              `json:"publicationDemographic"`
			Status                 string              `json:"status"`
			Year                   int                 `json:"year"`
			ContentRating          string              `json:"contentRating"`
			Tags                   []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					Name        map[string]string `json:"name"`
					Description map[string]string `json:"description"`
					Group       string            `json:"group"`
					Version     int               `json:"version"`
				}
				Relationships []string `json:"relationships"` // TODO: unsure about type
			} `json:"tags"`
			State                          string    `json:"state"`
			ChapterNumbersResetOnNewVolume bool      `json:"chapterNumbersResetOnNewVolume"`
			CreatedAt                      string    `json:"createdAt"`
			UpdatedAt                      string    `json:"updatedAt"`
			Version                        int       `json:"version"`
			AvailableTranslatedLanguages   []string  `json:"availableTranslatedLanguages"`
			LatestUploadedChapter          uuid.UUID `json:"latestUploadedChapter"`
		} `json:"attributes"`
		Relationships []struct {
			ID      uuid.UUID `json:"id"`
			Type    string    `json:"type"`
			Related string    `json:"related"`
		} `json:"relationships"`
	} `json:"data"`
}

// FetchTitle returns the title of the manga
func (m *Mangadex) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	requestURL, err := url.Parse("https://api.mangadex.org/")
	if err != nil {
		return "", err
	}

	id, err := getUUID(m.URL)
	if err != nil {
		return "", err
	}
	requestURL = requestURL.JoinPath("manga", id.String())
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL.String(), http.NoBody)
	if err != nil {
		return "", err
	}

	request.Header.Add("Referer", m.BaseURL())

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	var apiResponse mangadexMangaSearchAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", err
	}
	m.title = apiResponse.Data.Attributes.Title["en"] // TODO: make this configurable

	return m.title, nil
}

type mangadexFeedAPIResponse struct {
	Result   string `json:"result"`
	Response string `json:"response"`
	Data     []struct {
		ID         uuid.UUID `json:"id"`
		Type       string    `json:"type"`
		Attributes struct {
			Volume             string `json:"volume"`
			Chapter            string `json:"chapter"`
			Title              string `json:"title"`
			TranslatedLanguage string `json:"translatedLanguage"`
			ExternalURL        string `json:"externalUrl"`
			PublishAt          string `json:"publishAt"`
			CreatedAt          string `json:"createdAt"`
			UpdatedAt          string `json:"updatedAt"`
			Pages              int64  `json:"pages"`
			Version            int    `json:"version"`
		} `json:"attributes"`
		Relationships []struct {
			ID   uuid.UUID `json:"id"`
			Type string    `json:"type"`
		} `json:"relationships"`
	} `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// FetchChapters returns the chapters of the manga
func (m Mangadex) FetchChapters() (Filterables, error) {
	id, err := getUUID(m.URL)
	if err != nil {
		return nil, err
	}

	offset := 0
	var chapters Filterables

	for {
		uri, err := url.Parse("https://api.mangadex.org/")
		if err != nil {
			slog.Error(err.Error())
			break
		}
		uri.Path = path.Join(uri.Path, "manga", id.String(), "feed")

		params := url.Values{}
		params.Add("limit", strconv.FormatInt(500, 10))
		params.Add("order[volume]", "asc")
		params.Add("order[chapter]", "asc")
		params.Add("offset", fmt.Sprint(offset))
		if m.Settings.Language != "" {
			params.Add("translatedLanguage[]", m.Settings.Language)
		}
		uri.RawQuery = params.Encode()

		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uri.String(), http.NoBody)
		if err != nil {
			slog.Error(err.Error())
			break
		}
		resp, err := http.DefaultClient.Do(request)
		if err != nil {
			slog.Error(err.Error())
			break
		}
		body := mangadexFeedAPIResponse{}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()

		if err != nil {
			slog.Error(err.Error())
			break
		}

		if body.Result != "ok" {
			slog.Error(fmt.Sprintf("error fetching chapters: %s", body.Response))
		}

		for _, c := range body.Data {
			num, _ := strconv.ParseFloat(c.Attributes.Chapter, 64)
			chapters = append(chapters, &MangadexChapter{
				Chapter: Chapter{
					Number:     num,
					Title:      c.Attributes.Title,
					Language:   c.Attributes.TranslatedLanguage,
					PagesCount: c.Attributes.Pages,
				},
				ID: c.ID.String(),
			})
		}

		if len(body.Data) == 0 {
			break
		}

		offset += len(body.Data)
	}

	return chapters, nil
}

type mangadexPagesFeedAPIResponse struct {
	Result  string `json:"result"`
	BaseURL string `json:"baseUrl"`

	Chapter struct {
		Hash      string   `json:"hash"`
		Data      []string `json:"data"`
		DataSaver []string `json:"dataSaver"`
	} `json:"chapter"`
}

// FetchChapter fetches a chapter and its pages
func (m Mangadex) FetchChapter(f Filterable) (*Chapter, error) {
	chap, _ := f.(*MangadexChapter)

	uri, err := url.Parse("https://api.mangadex.org/at-home/server/")
	if err != nil {
		return nil, err
	}

	uri = uri.JoinPath(chap.ID)

	resp, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uri.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	// parse json body
	body := mangadexPagesFeedAPIResponse{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      fmt.Sprintf("Chapter %04d %s", int64(f.GetNumber()), chap.Title),
		Number:     f.GetNumber(),
		PagesCount: int64(len(body.Chapter.Data)),
		Language:   chap.Language,
	}

	baseURL, err := url.Parse(body.BaseURL)
	if err != nil {
		return nil, err
	}

	baseURL = baseURL.JoinPath("data", body.Chapter.Hash)

	// create pages
	for i, pageURL := range body.Chapter.Data {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    baseURL.JoinPath(pageURL).String(),
		})
	}

	return chapter, nil
}
