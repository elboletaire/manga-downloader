// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ranges"
)

// VolumesAuto is the --volumes value that groups chapters by the volumes the
// site files them under
const VolumesAuto = "auto"

// ErrNoVolumes is returned by GroupVolumes for --volumes auto when none of
// the chapters says which volume it's in
var ErrNoVolumes = errors.New(`the site doesn't say which volume these chapters are in; give the volumes as chapter ranges instead, e.g. "1-7;8-16"`)

// errVolumesBundle is the conflict between --volumes and --bundle
var errVolumesBundle = errors.New("can't be combined with --bundle")

// VolumeSpec is a parsed --volumes value
type VolumeSpec struct {
	// Auto groups chapters by the site's volumes
	Auto bool
	// Groups are the chapter ranges of each volume, volume N being Groups[N-1]
	Groups [][]ranges.Range
}

// ParseVolumes parses a --volumes value: "auto", or chapter ranges separated
// by semicolons, one group per volume. An empty value is a nil spec, packing
// per chapter.
func ParseVolumes(s string) (*VolumeSpec, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.EqualFold(s, VolumesAuto) {
		return &VolumeSpec{Auto: true}, nil
	}
	groups, err := ranges.ParseVolumes(s)
	if err != nil {
		return nil, err
	}
	return &VolumeSpec{Groups: groups}, nil
}

// Volume is one volume file of a --volumes run
type Volume struct {
	// Number is the volume's number, which names its file
	Number int
	// Chapters are the listed chapters it holds, sorted by number
	Chapters grabber.Filterables
}

// FirstNumber and LastNumber are the volume's lowest and highest chapter
// numbers; Chapters is never empty
func (v *Volume) FirstNumber() float64 { return v.Chapters[0].GetNumber() }
func (v *Volume) LastNumber() float64  { return v.Chapters[len(v.Chapters)-1].GetNumber() }

// GroupVolumes splits chapters into the spec's volumes, sorted by number, and
// the chapters no volume holds, which are packed one file per chapter rather
// than dropped. A volume holding none of the chapters isn't returned. With
// Auto, ErrNoVolumes is returned when no chapter has a volume.
func GroupVolumes(chapters grabber.Filterables, spec *VolumeSpec) ([]*Volume, grabber.Filterables, error) {
	byNumber := map[int]*Volume{}
	var loose grabber.Filterables
	for _, chap := range chapters {
		n := 0
		if spec.Auto {
			n = grabber.VolumeOf(chap)
		} else {
			n = groupOf(chap.GetNumber(), spec.Groups)
		}
		if n <= 0 {
			loose = append(loose, chap)
			continue
		}
		v := byNumber[n]
		if v == nil {
			v = &Volume{Number: n}
			byNumber[n] = v
		}
		v.Chapters = append(v.Chapters, chap)
	}
	if spec.Auto && len(byNumber) == 0 {
		return nil, nil, ErrNoVolumes
	}

	volumes := make([]*Volume, 0, len(byNumber))
	for _, v := range byNumber {
		// a copy, so sorting doesn't reorder the caller's slice
		v.Chapters = append(grabber.Filterables(nil), v.Chapters...).SortByNumber()
		volumes = append(volumes, v)
	}
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Number < volumes[j].Number })

	return volumes, loose, nil
}

// groupOf returns the 1-based group holding number, 0 when none does
func groupOf(number float64, groups [][]ranges.Range) int {
	for i, g := range groups {
		for _, r := range g {
			if number >= r.Begin && number <= r.End {
				return i + 1
			}
		}
	}
	return 0
}

// SiteVolumes writes the volumes the site files chapters under as a --volumes
// value ("1-7;8-16"), so the desktop app can prefill its box with something
// that can be edited. chapters must be sorted by number. Each group lists
// runs of consecutive chapters, so a chapter of another volume (or of none)
// between two of a volume's never gets swept into its range. A volume number
// with no chapters keeps its place as an empty group. It's "" when no chapter
// has a volume.
func SiteVolumes(chapters grabber.Filterables) string {
	// the volume of each listed position; a number listed more than once (one
	// entry per language, say) belongs to the first volume it shows up in,
	// since groups can't overlap
	volumeAt := make([]int, len(chapters))
	owner := map[float64]int{}
	maxVolume := 0
	for i, chap := range chapters {
		v := grabber.VolumeOf(chap)
		if v <= 0 {
			continue
		}
		if o, ok := owner[chap.GetNumber()]; ok && o != v {
			continue
		}
		owner[chap.GetNumber()] = v
		volumeAt[i] = v
		if v > maxVolume {
			maxVolume = v
		}
	}
	if maxVolume == 0 {
		return ""
	}

	groups := make([][]string, maxVolume)
	for i := 0; i < len(chapters); {
		v := volumeAt[i]
		if v == 0 {
			i++
			continue
		}
		j := i
		// a repeated number of the same volume doesn't break the run
		for j+1 < len(chapters) && (volumeAt[j+1] == v || (volumeAt[j+1] == 0 && owner[chapters[j+1].GetNumber()] == v)) {
			j++
		}
		r := ranges.Range{Begin: chapters[i].GetNumber(), End: chapters[j].GetNumber()}
		groups[v-1] = append(groups[v-1], r.String())
		i = j + 1
	}

	parts := make([]string, len(groups))
	for i, g := range groups {
		parts[i] = strings.Join(g, ",")
	}
	return strings.Join(parts, "; ")
}

// VolumeLabel is how a volume is shown in progress and messages: "Vol. 01"
func VolumeLabel(n int) string {
	s := strconv.Itoa(n)
	if n < 10 {
		s = "0" + s
	}
	return "Vol. " + s
}
