# Volume Batching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to batch downloaded chapters into per-volume CBZ files by specifying chapter ranges via `--volumes` flag or `--volume-file`.

**Architecture:** Two new parsing functions in `ranges`, a new `Volume` field in `packer.FilenameTemplateParts` plus a `PackVolume` function, and a new volume download loop in `cmd/root.go` that reuses existing concurrency and packing infrastructure.

**Tech Stack:** Go, goquery, mpb/v8 (progress bars), cobra (CLI flags)

---

### Task 1: ParseVolumes

**Files:**
- Modify: `ranges/parser.go`
- Modify: `ranges/parser_test.go`

- [ ] **Step 1: Write the failing test**

Append to `ranges/parser_test.go`:

```go
func TestParseVolumes(t *testing.T) {
	// basic three-volume split
	vols, err := ParseVolumes("1-8;9-17;18-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(vols) != 3 {
		t.Fatalf("expected 3 volumes, got %d", len(vols))
	}
	if vols[0][0].Begin != 1 || vols[0][0].End != 8 {
		t.Error("expected volume 1 range 1-8")
	}
	if vols[1][0].Begin != 9 || vols[1][0].End != 17 {
		t.Error("expected volume 2 range 9-17")
	}
	if vols[2][0].Begin != 18 || vols[2][0].End != 25 {
		t.Error("expected volume 3 range 18-25")
	}

	// decimal chapter numbers
	vols2, err := ParseVolumes("168.1-170;262.5")
	if err != nil {
		t.Fatal(err)
	}
	if vols2[0][0].Begin != 168.1 || vols2[0][0].End != 170 {
		t.Error("expected volume 1 range 168.1-170")
	}
	if vols2[1][0].Begin != 262.5 || vols2[1][0].End != 262.5 {
		t.Error("expected volume 2 range 262.5-262.5")
	}

	// comma-separated chapters within a volume
	vols3, err := ParseVolumes("1-8,10;9-17")
	if err != nil {
		t.Fatal(err)
	}
	if len(vols3[0]) != 2 {
		t.Errorf("expected volume 1 to have 2 ranges, got %d", len(vols3[0]))
	}
	if vols3[0][1].Begin != 10 || vols3[0][1].End != 10 {
		t.Error("expected second range of volume 1 to be 10-10")
	}

	// invalid range returns error
	_, err = ParseVolumes("abc")
	if err == nil {
		t.Error("expected error for invalid range")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test -v ./ranges/... -run TestParseVolumes
```

Expected: `FAIL` — `ParseVolumes` undefined.

- [ ] **Step 3: Implement ParseVolumes**

Add to the bottom of `ranges/parser.go`:

```go
// ParseVolumes parses a semicolon-delimited string of chapter ranges into
// a slice of range-slices, where each inner slice represents one volume.
// Commas within a segment work the same as in Parse (e.g. "1-8,10" is valid).
func ParseVolumes(s string) ([][]Range, error) {
	segments := strings.Split(s, ";")
	volumes := make([][]Range, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		rngs, err := Parse(seg)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, rngs)
	}
	return volumes, nil
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
go test -v ./ranges/... -run TestParseVolumes
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add ranges/parser.go ranges/parser_test.go
git commit -m "feat(ranges): add ParseVolumes for semicolon-delimited volume ranges"
```

---

### Task 2: ParseVolumesFile

**Files:**
- Modify: `ranges/parser.go`
- Modify: `ranges/parser_test.go`

- [ ] **Step 1: Write the failing test**

Append to `ranges/parser_test.go`. Add `"os"` to the import (change `import "testing"` to `import ("os" "testing")`):

```go
func TestParseVolumesFile(t *testing.T) {
	// write a temp file with comments, blank lines, and decimal values
	content := "# Volume 1\n1-8\n# Volume 2\n9-17\n\n18-25\n"
	f, err := os.CreateTemp("", "volumes*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	vols, err := ParseVolumesFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	// 3 non-blank non-comment lines → 3 volumes
	if len(vols) != 3 {
		t.Fatalf("expected 3 volumes, got %d", len(vols))
	}
	if vols[0][0].Begin != 1 || vols[0][0].End != 8 {
		t.Error("expected volume 1 range 1-8")
	}
	if vols[1][0].Begin != 9 || vols[1][0].End != 17 {
		t.Error("expected volume 2 range 9-17")
	}
	if vols[2][0].Begin != 18 || vols[2][0].End != 25 {
		t.Error("expected volume 3 range 18-25")
	}

	// decimal chapter numbers
	f2, err := os.CreateTemp("", "volumes*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f2.Name())
	f2.WriteString("168.1-170\n262.5\n")
	f2.Close()

	vols2, err := ParseVolumesFile(f2.Name())
	if err != nil {
		t.Fatal(err)
	}
	if vols2[0][0].Begin != 168.1 || vols2[0][0].End != 170 {
		t.Error("expected volume 1 range 168.1-170")
	}
	if vols2[1][0].Begin != 262.5 || vols2[1][0].End != 262.5 {
		t.Error("expected volume 2 range 262.5-262.5")
	}

	// missing file returns error
	_, err = ParseVolumesFile("/nonexistent/volumes.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test -v ./ranges/... -run TestParseVolumesFile
```

Expected: `FAIL` — `ParseVolumesFile` undefined.

- [ ] **Step 3: Implement ParseVolumesFile**

Add `"os"` to the imports in `ranges/parser.go` (the existing import is just `"strconv"` and `"strings"` — add `"os"`).

Then append to `ranges/parser.go`:

```go
// ParseVolumesFile reads a plain-text file where each non-blank, non-comment
// line is a chapter range (parsed by Parse). Lines starting with "#" are
// treated as comments and skipped. Each line becomes one volume.
func ParseVolumesFile(path string) ([][]Range, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	volumes := make([][]Range, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rngs, err := Parse(line)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, rngs)
	}
	return volumes, nil
}
```

- [ ] **Step 4: Run all range tests to confirm they pass**

```bash
go test -v ./ranges/...
```

Expected: all tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add ranges/parser.go ranges/parser_test.go
git commit -m "feat(ranges): add ParseVolumesFile for file-based volume ranges"
```

---

### Task 3: Volume filename support and PackVolume

**Files:**
- Modify: `packer/filename.go`
- Create: `packer/filename_test.go`
- Modify: `packer/pack.go`

- [ ] **Step 1: Write the failing filename test**

Create `packer/filename_test.go`:

```go
package packer

import "testing"

func TestVolumeFilenameTemplate(t *testing.T) {
	parts := FilenameTemplateParts{
		Series: "One Piece",
		Volume: 1,
	}
	got, err := NewFilenameFromTemplate(FilenameVolumeTemplateDefault, parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "One Piece Vol.01" {
		t.Errorf("expected 'One Piece Vol.01', got '%s'", got)
	}

	// double-digit volume
	parts.Volume = 12
	got, err = NewFilenameFromTemplate(FilenameVolumeTemplateDefault, parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "One Piece Vol.12" {
		t.Errorf("expected 'One Piece Vol.12', got '%s'", got)
	}

	// version suffix when Version > 1 (duplicate filename handling)
	parts.Volume = 1
	parts.Version = 2
	got, err = NewFilenameFromTemplate(FilenameVolumeTemplateDefault, parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "One Piece Vol.01 v2" {
		t.Errorf("expected 'One Piece Vol.01 v2', got '%s'", got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test -v ./packer/... -run TestVolumeFilenameTemplate
```

Expected: `FAIL` — `FilenameVolumeTemplateDefault` undefined.

- [ ] **Step 3: Add Volume field and default template to packer/filename.go**

Add `Volume int` to `FilenameTemplateParts` and a new constant. The full updated file:

```go
package packer

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/elboletaire/manga-downloader/grabber"
)

// FilenameTemplateParts represents the parts of a filename
type FilenameTemplateParts struct {
	// Series represents the series name (e.g. "One Piece")
	Series string
	// Number represents the chapter number (e.g. "1.0")
	Number string
	// Title represents the chapter title (e.g. "The Beginning")
	Title string
	// Volume represents the volume number (e.g. 1)
	Volume int
	// Version placeholder appended to the title in case of duplicate filenames (e.g. "3")
	Version int
}

// FilenameTemplateDefault is the default filename template for individual chapters
const FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// FilenameVolumeTemplateDefault is the default filename template for volume CBZ files
const FilenameVolumeTemplateDefault = `{{.Series}} Vol.{{printf "%02d" .Volume}}{{if gt .Version 1}} v{{.Version}}{{end}}`

// NewFilenameFromTemplate returns a new filename from a series title, a chapter and a template
func NewFilenameFromTemplate(templ string, parts FilenameTemplateParts) (string, error) {
	tmpl, err := template.New("filename").Parse(templ)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, parts)

	return buffer.String(), err
}

// NewChapterFileTemplateParts returns a new FilenameTemplateParts from a title and a chapter
func NewChapterFileTemplateParts(title string, chapter *grabber.Chapter) FilenameTemplateParts {
	return FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Number: strings.Replace(fmt.Sprintf("%.1f", chapter.GetNumber()), ".0", "", 1),
		Title:  SanitizeFilename(chapter.GetTitle()),
	}
}

// SanitizeFilename sanitizes a filename
func SanitizeFilename(filename string) string {
	sanitized := strings.Replace(filename, "/", "_", -1)
	sanitized = strings.Replace(sanitized, "\\", "_", -1)
	sanitized = strings.Replace(sanitized, ":", ";", -1)
	sanitized = strings.Replace(sanitized, "?", "¿", -1)
	sanitized = strings.Replace(sanitized, `"`, "'", -1)

	return sanitized
}
```

- [ ] **Step 4: Run the filename test to confirm it passes**

```bash
go test -v ./packer/... -run TestVolumeFilenameTemplate
```

Expected: `PASS`

- [ ] **Step 5: Add PackVolume to packer/pack.go**

Append to `packer/pack.go` (after the existing `PackBundle` function):

```go
// PackVolume packs a set of downloaded chapters into a single volume CBZ file.
// volNum is the 1-based volume index used in the output filename.
func PackVolume(outputdir, tmpl string, s grabber.Site, chapters []*DownloadedChapter, volNum int, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	files := []*downloader.File{}
	for _, chapter := range chapters {
		files = append(files, chapter.Files...)
	}
	return pack(outputdir, tmpl, title, FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Volume: volNum,
	}, files, progress)
}
```

- [ ] **Step 6: Build to confirm no compilation errors**

```bash
go build ./...
```

Expected: no output (success).

- [ ] **Step 7: Commit**

```bash
git add packer/filename.go packer/filename_test.go packer/pack.go
git commit -m "feat(packer): add Volume field, FilenameVolumeTemplateDefault, and PackVolume"
```

---

### Task 4: CLI flags and validation

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Add package-level vars and flags**

In `cmd/root.go`, add three new package-level variables alongside the existing `var settings grabber.Settings`:

```go
var volumesStr string
var volumeFile string
var volumeFilenameTemplate string
```

In the `init()` function, append after the existing flag declarations:

```go
rootCmd.Flags().StringVar(&volumesStr, "volumes", "", `semicolon-delimited chapter ranges, one per volume (e.g. "1-8;9-17;18-25")`)
rootCmd.Flags().StringVar(&volumeFile, "volume-file", "", "path to a file with one chapter range per line, each line becoming one volume")
rootCmd.Flags().StringVar(&volumeFilenameTemplate, "volume-filename-template", packer.FilenameVolumeTemplateDefault, "template for volume output filenames")
```

- [ ] **Step 2: Add mutual-exclusion validation**

In `Run`, directly after the `s.InitFlags(cmd)` call, add:

```go
if volumesStr != "" && volumeFile != "" {
    cerr(fmt.Errorf("--volumes and --volume-file are mutually exclusive"), "")
}
if (volumesStr != "" || volumeFile != "") && settings.Bundle {
    cerr(fmt.Errorf("--volumes/--volume-file and --bundle are mutually exclusive"), "")
}
if (volumesStr != "" || volumeFile != "") && len(args) > 1 {
    cerr(fmt.Errorf("--volumes/--volume-file and a chapter range argument are mutually exclusive"), "")
}
```

- [ ] **Step 3: Build to confirm no compilation errors**

```bash
go build ./...
```

Expected: no output (success).

- [ ] **Step 4: Smoke-test flag registration**

```bash
go run . --help
```

Expected: `--volumes`, `--volume-file`, and `--volume-filename-template` appear in the flags list.

- [ ] **Step 5: Commit**

```bash
git add cmd/root.go
git commit -m "feat(cmd): add --volumes, --volume-file, --volume-filename-template flags"
```

---

### Task 5: Volume download loop

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Add the volume download loop to Run**

In `Run`, after the `chapters = chapters.SortByNumber()` line and before the existing range-parsing block (the `var rngs []ranges.Range` line), insert the volume handling block:

```go
// volume mode: download and pack chapters into per-volume CBZ files
if volumesStr != "" || volumeFile != "" {
    var volumeRanges [][]ranges.Range
    if volumesStr != "" {
        volumeRanges, err = ranges.ParseVolumes(volumesStr)
    } else {
        volumeRanges, err = ranges.ParseVolumesFile(volumeFile)
    }
    cerr(err, "Error parsing volumes: ")

    for i, volRanges := range volumeRanges {
        volNum := i + 1

        volChapters := chapters.FilterRanges(volRanges)
        if len(volChapters) == 0 {
            color.Yellow("Vol.%02d: no chapters found in specified range, skipping", volNum)
            continue
        }

        // pre-fetch page counts for progress bar total
        totalPages := int64(0)
        for _, chap := range volChapters {
            chapter, err := s.FetchChapter(chap)
            if err == nil && chapter != nil {
                totalPages += chapter.PagesCount
            }
        }

        // progress bar for this volume
        p := mpb.New(
            mpb.WithWidth(40),
            mpb.WithOutput(color.Output),
            mpb.WithAutoRefresh(),
        )
        volLabel := fmt.Sprintf("Vol.%02d", volNum)
        bar := p.AddBar(totalPages*2,
            mpb.PrependDecorators(
                decor.Any(func(s decor.Statistics) string {
                    return blue.Sprintf("%-30s", volLabel)
                }, decor.WCSyncWidthR),
                decor.CountersNoUnit("%d/%d", decor.WC{C: decor.DextraSpace}),
            ),
            mpb.AppendDecorators(
                decor.Percentage(decor.WC{W: 4}),
                decor.Any(func(s decor.Statistics) string {
                    if s.Current >= s.Total {
                        return blue.Sprintf(" bundling ")
                    }
                    return blue.Sprintf(" downloading")
                }, decor.WC{W: 10}),
            ),
        )

        // download chapters concurrently
        wg := sync.WaitGroup{}
        g := make(chan struct{}, s.GetMaxConcurrency().Chapters)
        downloaded := grabber.Filterables{}

        for _, chap := range volChapters {
            g <- struct{}{}
            wg.Add(1)
            go func(chap grabber.Filterable) {
                defer wg.Done()
                chapter, err := s.FetchChapter(chap)
                if err != nil {
                    color.Red("- error fetching chapter %s: %s", chap.GetTitle(), err.Error())
                    <-g
                    return
                }
                files, err := downloader.FetchChapter(s, chapter, func(_ int, idx int, err error) {
                    if err != nil {
                        color.Red("- error downloading page %d: %s", idx+1, err.Error())
                    } else {
                        bar.IncrBy(1)
                    }
                })
                if err != nil {
                    color.Red("- error downloading chapter %s: %s", chapter.GetTitle(), err.Error())
                    <-g
                    return
                }
                bar.IncrBy(int(chapter.PagesCount))
                downloaded = append(downloaded, &packer.DownloadedChapter{
                    Chapter: chapter,
                    Files:   files,
                })
                <-g
            }(chap)
        }
        wg.Wait()
        close(g)
        p.Wait()

        // sort and convert for packing
        downloaded = downloaded.SortByNumber()
        dc := make([]*packer.DownloadedChapter, 0, len(downloaded))
        for _, d := range downloaded {
            dc = append(dc, d.(*packer.DownloadedChapter))
        }

        filename, err := packer.PackVolume(settings.OutputDir, volumeFilenameTemplate, s, dc, volNum, func(_, _ int) {})
        if err != nil {
            color.Red(err.Error())
            continue
        }
        fmt.Printf("- %s %s\n", color.GreenString("saved file"), color.HiBlackString(filename))
    }
    os.Exit(0)
}
```

Note: `err` is already declared earlier in `Run` (from `title, err := s.FetchTitle()`), so use `=` not `:=` when assigning `volumeRanges, err`.

- [ ] **Step 2: Build to confirm no compilation errors**

```bash
go build ./...
```

Expected: no output (success).

- [ ] **Step 3: Run all tests**

```bash
go test -v ./...
```

Expected: all tests `PASS`.

- [ ] **Step 4: Smoke-test with a real URL (optional but recommended)**

```bash
go run . --volumes "1-3;4-6" https://mangabuddy.com/jujutsu-kaisen
```

Expected: two files created — `Jujutsu Kaisen Vol.01.cbz` (chapters 1–3) and `Jujutsu Kaisen Vol.02.cbz` (chapters 4–6).

Test `--volume-file`:
```
# volumes.txt
1-3
4-6
```
```bash
go run . --volume-file volumes.txt https://mangabuddy.com/jujutsu-kaisen
```

Expected: same two files as above.

- [ ] **Step 5: Commit**

```bash
git add cmd/root.go
git commit -m "feat(cmd): add volume download loop for --volumes and --volume-file"
```

- [ ] **Step 6: Push**

```bash
git push origin master
```
