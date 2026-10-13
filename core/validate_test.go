// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

func TestValidateFormatAndReadingDirection(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		direction string
		setting   string // the SettingError it should fail with; "" when it passes
	}{
		{"cbz", packer.FormatCBZ, packer.ReadingDirectionRTL, ""},
		{"raw", packer.FormatRaw, packer.ReadingDirectionLTR, ""},
		{"epub rtl", packer.FormatEPUB, packer.ReadingDirectionRTL, ""},
		{"epub ltr", packer.FormatEPUB, packer.ReadingDirectionLTR, ""},
		{"epub without a direction", packer.FormatEPUB, "", ""},
		{"unknown format", "pdf", packer.ReadingDirectionRTL, "format"},
		{"unknown direction", packer.FormatEPUB, "ttb", "reading-direction"},
		{"direction case matters", packer.FormatEPUB, "RTL", "reading-direction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &grabber.Settings{Format: tt.format, ReadingDirection: tt.direction, ConvertImages: grabber.ConvertImagesDefault}
			_, err := Validate(s)
			if tt.setting == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want ok", err)
				}
				return
			}
			var se *SettingError
			if !errors.As(err, &se) || se.Setting != tt.setting {
				t.Fatalf("Validate() = %v, want a %s SettingError", err, tt.setting)
			}
		})
	}
}
