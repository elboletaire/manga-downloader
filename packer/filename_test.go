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
