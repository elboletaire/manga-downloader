package ranges

import "testing"

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
