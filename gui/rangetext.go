// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"strconv"
	"strings"
)

// bundleRange compacts the selected chapters into the range text a bundle is
// named after ("1-3,5,7.5"). numbers are the listed chapter numbers, sorted,
// and selected flags each of them. A run is a stretch of consecutive listed
// chapters that are all selected, so a chapter the list doesn't select breaks
// the run even when the numbers on either side of it are selected.
func bundleRange(numbers []float64, selected []bool) string {
	var parts []string
	for i := 0; i < len(numbers); {
		if !selected[i] {
			i++
			continue
		}
		j := i
		for j+1 < len(numbers) && selected[j+1] {
			j++
		}
		parts = append(parts, formatRun(numbers[i], numbers[j]))
		i = j + 1
	}

	return strings.Join(parts, ",")
}

func formatRun(begin, end float64) string {
	if begin == end {
		return formatNumber(begin)
	}
	return formatNumber(begin) + "-" + formatNumber(end)
}

func formatNumber(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
