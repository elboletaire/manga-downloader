# Volume Batching Design

## Overview

Add support for batching downloaded chapters into per-volume CBZ files. Volume boundaries are always user-specified — either via a CLI flag or a file. When no volumes are specified, all existing behavior is unchanged.

## CLI

Two new flags:

- `--volumes "1-8;9-17;18-25"` — semicolon-delimited list of chapter ranges, one per volume. Commas within a segment work as they do today (e.g. `1-8,10` is valid).
- `--volume-file path/to/file.txt` — path to a plain text file with one range per line.

One new optional flag:

- `--volume-filename-template` — Go template string for volume output filenames. Defaults to `{{.Series}} Vol.{{printf "%02d" .Volume}}`.

Constraints (any violation is a fatal error with a clear message):

- `--volumes` and `--volume-file` are mutually exclusive.
- `--volumes`/`--volume-file` and `--bundle` are mutually exclusive.
- `--volumes`/`--volume-file` and a positional chapter range argument are mutually exclusive.

## Range Parsing

Two new functions added to the `ranges` package:

```go
// ParseVolumes splits on ";" and calls Parse() on each segment.
// Returns a slice of range-slices; each inner slice is one volume.
func ParseVolumes(s string) ([][]Range, error)

// ParseVolumesFile reads a file line by line, skipping blank lines
// and lines starting with "#", and calls Parse() on each line.
func ParseVolumesFile(path string) ([][]Range, error)
```

Both delegate to the existing `Parse()` which uses `float64` internally, so decimal chapter numbers (e.g. `168.1`, `262.5`) are supported with no extra work.

Example volume file format:

```
# Volume 1
1-8
# Volume 2
9-17,19
# Volume 3
20-25.5
```

Volume numbers are positional — the first entry is Vol.1, the second is Vol.2, etc.

## Output Filenames

Add `Volume int` to `FilenameTemplateParts` in `packer/filename.go`.

New default volume template constant:

```go
const FilenameVolumeTemplateDefault = `{{.Series}} Vol.{{printf "%02d" .Volume}}`
```

Produces e.g. `One Piece Vol.01.cbz`, `One Piece Vol.02.cbz`. Zero-padding to 2 digits handles up to 99 volumes and ensures correct lexicographic sort order in file browsers.

## Download Flow

When volumes are active, `cmd/root.go`:

1. Fetches and sorts all chapters from the site (same as today).
2. Iterates over volumes **sequentially**, one at a time.
3. For each volume:
   - Filters the full chapter list to the volume's ranges.
   - Downloads all chapters in the volume concurrently (same goroutine/semaphore logic as today).
   - Packs into a single CBZ via the existing `PackBundle`, passing the volume number for the filename.
4. Progress bar labels include the current volume (e.g. `Vol.01 - Chapter 5:`).

Volumes are processed sequentially rather than in parallel to avoid hammering the server. Per-chapter concurrency within each volume is unchanged.

## Files Changed

- `ranges/parser.go` — add `ParseVolumes` and `ParseVolumesFile`
- `ranges/parser_test.go` — add tests for both new functions, including decimal values
- `packer/filename.go` — add `Volume int` to `FilenameTemplateParts`, add `FilenameVolumeTemplateDefault`
- `cmd/root.go` — add `--volumes`, `--volume-file`, `--volume-filename-template` flags; add volume download loop
