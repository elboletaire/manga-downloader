package ranges

import (
	"os"
	"testing"
)

func TestRangesParsing(t *testing.T) {
	rngs, err := Parse("1-20,23,55-1059")
	if err != nil {
		t.Error(err)
	}
	if len(rngs) != 3 {
		t.Error("Expected 3 ranges")
	}
	if rngs[0].Begin != 1 || rngs[0].End != 20 {
		t.Error("Expected range 1-20")
	}
	if rngs[1].Begin != 23 || rngs[1].End != 23 {
		t.Error("Expected range 23-23")
	}
	if rngs[2].Begin != 55 || rngs[2].End != 1059 {
		t.Error("Expected range 55-1059")
	}
}

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
