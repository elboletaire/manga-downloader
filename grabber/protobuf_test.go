// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// The helpers below hand-encode protobuf messages, so the fixtures in these
// tests (and the MangaPlus ones) are built field by field out of the schema
// instead of being opaque blobs.

// pbTestVarint encodes a varint field
func pbTestVarint(num int, value uint64) []byte {
	return append(pbTestTag(num, pbWireVarint), pbTestBytesOf(value)...)
}

// pbTestString encodes a length-delimited field holding a string
func pbTestString(num int, value string) []byte {
	return pbTestBytes(num, []byte(value))
}

// pbTestBytes encodes a length-delimited field holding raw bytes (an embedded
// message, in practice)
func pbTestBytes(num int, value []byte) []byte {
	field := append(pbTestTag(num, pbWireBytes), pbTestBytesOf(uint64(len(value)))...)

	return append(field, value...)
}

// pbTestFixed64 encodes a fixed64 field
func pbTestFixed64(num int, value uint64) []byte {
	field := append(pbTestTag(num, pbWireFixed64), make([]byte, 8)...)
	binary.LittleEndian.PutUint64(field[len(field)-8:], value)

	return field
}

// pbTestFixed32 encodes a fixed32 field
func pbTestFixed32(num int, value uint32) []byte {
	field := append(pbTestTag(num, pbWireFixed32), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(field[len(field)-4:], value)

	return field
}

// pbTestTag encodes a field's key: its number shifted left by three bits, plus
// its wire type. Field numbers past 15 don't fit in the low 7 bits, so their
// key takes a second byte.
func pbTestTag(num, wire int) []byte {
	return pbTestBytesOf(uint64(num)<<3 | uint64(wire))
}

// pbTestBytesOf encodes a value as a base-128 varint
func pbTestBytesOf(value uint64) []byte {
	var encoded []byte

	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		encoded = append(encoded, b)
		if value == 0 {
			return encoded
		}
	}
}

// pbTestConcat joins the encoded fields of one message
func pbTestConcat(fields ...[]byte) []byte {
	return bytes.Join(fields, nil)
}

func TestPbParseVarints(t *testing.T) {
	// 300 and 1<<35 need two and five bytes respectively
	data := pbTestConcat(
		pbTestVarint(1, 1),
		pbTestVarint(2, 300),
		pbTestVarint(3, 1<<35),
		pbTestVarint(14, 0),
	)

	fields, err := pbParse(data)
	if err != nil {
		t.Fatalf("pbParse: %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("got %d fields, want 4", len(fields))
	}

	for i, num := range []int{1, 2, 3, 14} {
		if fields[i].num != num || fields[i].wire != pbWireVarint {
			t.Errorf("field %d = {num:%d wire:%d}, want {num:%d wire:%d}", i, fields[i].num, fields[i].wire, num, pbWireVarint)
		}
	}

	if got := pbUint(fields, 2); got != 300 {
		t.Errorf("pbUint(2) = %d, want 300", got)
	}
	if got := pbUint(fields, 3); got != 1<<35 {
		t.Errorf("pbUint(3) = %d, want %d", got, 1<<35)
	}
	if got := pbUint(fields, 1); got != 1 {
		t.Errorf("pbUint(1) = %d, want 1", got)
	}
	if got := pbUint(fields, 14); got != 0 {
		t.Errorf("pbUint(14) = %d, want 0", got)
	}
}

func TestPbParseLengthDelimitedAndNested(t *testing.T) {
	nested := pbTestConcat(pbTestVarint(1, 7), pbTestString(2, "inner"))
	data := pbTestConcat(
		pbTestString(1, "first"),
		pbTestString(1, "second"),
		pbTestBytes(3, nested),
	)

	fields, err := pbParse(data)
	if err != nil {
		t.Fatalf("pbParse: %v", err)
	}

	repeated := pbRepeated(fields, 1)
	if len(repeated) != 2 {
		t.Fatalf("got %d fields numbered 1, want 2", len(repeated))
	}
	if repeated[0].String() != "first" || repeated[1].String() != "second" {
		t.Errorf("repeated fields = %q, %q, want %q, %q", repeated[0].String(), repeated[1].String(), "first", "second")
	}

	// pbFieldByNumber answers with the last field of that number, which is the
	// one that counts for a singular field
	if got := pbString(fields, 1); got != "second" {
		t.Errorf("pbString(1) = %q, want %q", got, "second")
	}

	message, ok := pbFieldByNumber(fields, 3)
	if !ok {
		t.Fatal("field 3 is missing")
	}
	inner, err := pbMessage(message)
	if err != nil {
		t.Fatalf("pbMessage: %v", err)
	}
	if got := pbUint(inner, 1); got != 7 {
		t.Errorf("inner field 1 = %d, want 7", got)
	}
	if got := pbString(inner, 2); got != "inner" {
		t.Errorf("inner field 2 = %q, want %q", got, "inner")
	}
}

// Field numbers past 15 take a two-byte key, which is what the MangaPlus
// chapter lists (28 and 38) use
func TestPbParseTwoByteFieldNumbers(t *testing.T) {
	data := pbTestConcat(
		pbTestString(15, "one byte key"),
		pbTestString(16, "two byte key"),
		pbTestString(28, "chapter list group"),
		pbTestString(38, "chapter list"),
	)

	fields, err := pbParse(data)
	if err != nil {
		t.Fatalf("pbParse: %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("got %d fields, want 4", len(fields))
	}

	for i, want := range []struct {
		num  int
		text string
	}{{15, "one byte key"}, {16, "two byte key"}, {28, "chapter list group"}, {38, "chapter list"}} {
		if fields[i].num != want.num {
			t.Errorf("field %d has number %d, want %d", i, fields[i].num, want.num)
		}
		if fields[i].String() != want.text {
			t.Errorf("field %d = %q, want %q", want.num, fields[i].String(), want.text)
		}
	}
}

// Unknown fields of every wire type must be skipped over without disturbing
// the ones around them: that's what makes a schema change survivable
func TestPbParseSkipsUnknownFields(t *testing.T) {
	data := pbTestConcat(
		pbTestVarint(1, 42),
		pbTestFixed64(100, 1<<40),
		pbTestString(600, "unknown length-delimited"),
		pbTestFixed32(700, 7),
		pbTestString(2, "kept"),
	)

	fields, err := pbParse(data)
	if err != nil {
		t.Fatalf("pbParse: %v", err)
	}
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	if got := pbUint(fields, 1); got != 42 {
		t.Errorf("pbUint(1) = %d, want 42", got)
	}
	if got := pbString(fields, 2); got != "kept" {
		t.Errorf("pbString(2) = %q, want %q", got, "kept")
	}

	fixed64, ok := pbFieldByNumber(fields, 100)
	if !ok || fixed64.wire != pbWireFixed64 || fixed64.Uint() != 1<<40 {
		t.Errorf("fixed64 field = %+v (ok=%v), want wire %d and value %d", fixed64, ok, pbWireFixed64, 1<<40)
	}
	fixed32, ok := pbFieldByNumber(fields, 700)
	if !ok || fixed32.wire != pbWireFixed32 || fixed32.Uint() != 7 {
		t.Errorf("fixed32 field = %+v (ok=%v), want wire %d and value 7", fixed32, ok, pbWireFixed32)
	}
	if got := pbString(fields, 600); got != "unknown length-delimited" {
		t.Errorf("pbString(600) = %q, want %q", got, "unknown length-delimited")
	}
}

func TestPbParseTruncated(t *testing.T) {
	complete := pbTestConcat(
		pbTestString(1, "a title"),
		pbTestVarint(2, 300),
		pbTestFixed64(3, 1),
		pbTestFixed32(4, 1),
	)

	cases := []struct {
		name string
		data []byte
	}{
		{"cut inside the key varint", pbTestConcat(pbTestTag(300, pbWireBytes), []byte{0x82})},
		{"cut before a varint value", pbTestTag(2, pbWireVarint)},
		{"cut inside a varint value", pbTestConcat(pbTestTag(2, pbWireVarint), []byte{0xac})},
		{"cut before a length", pbTestTag(1, pbWireBytes)},
		{"cut inside a length", pbTestConcat(pbTestTag(1, pbWireBytes), []byte{0x90})},
		{"cut inside a payload", pbTestConcat(pbTestTag(1, pbWireBytes), []byte{0x08, 'a', 'b'})},
		{"cut inside a fixed64", pbTestConcat(pbTestTag(3, pbWireFixed64), []byte{1, 2, 3})},
		{"cut inside a fixed32", pbTestConcat(pbTestTag(4, pbWireFixed32), []byte{1, 2})},
		{"truncated after a complete field", complete[:len(complete)-2]},
	}

	for _, c := range cases {
		if _, err := pbParse(c.data); err == nil {
			t.Errorf("%s: pbParse returned no error", c.name)
		}
	}
}

// An empty message is a valid one (it simply has no fields); it's a truncated
// one that must not be mistaken for it
func TestPbParseEmptyMessage(t *testing.T) {
	for _, data := range [][]byte{nil, {}} {
		fields, err := pbParse(data)
		if err != nil {
			t.Fatalf("pbParse(%v): %v", data, err)
		}
		if len(fields) != 0 {
			t.Errorf("pbParse(%v) = %d fields, want 0", data, len(fields))
		}
	}
}

func TestPbParseRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		// field number 0 is invalid on the wire
		{"field number zero", []byte{0x00, 0x01}},
		// 11 continuation bytes can't fit in a uint64
		{"varint longer than 10 bytes", append([]byte{0x08}, bytes.Repeat([]byte{0x80}, 10)...)},
		// wire types 3 and 4 are the deprecated groups, and 6 and 7 don't exist
		{"group start", []byte{0x0b}},
		{"group end", []byte{0x0c}},
		{"invalid wire type", []byte{0x0e}},
	}

	for _, c := range cases {
		if _, err := pbParse(c.data); err == nil {
			t.Errorf("%s: pbParse returned no error", c.name)
		}
	}
}

func TestPbMessageOnNonMessage(t *testing.T) {
	if _, err := pbMessage(pbField{num: 1, wire: pbWireVarint}); err == nil {
		t.Error("pbMessage accepted a varint field")
	}
}

// Absent fields read as their type's default value, which is how protobuf
// leaves them out of the wire (an enum's first value, in particular)
func TestPbFieldLookupsWhenAbsent(t *testing.T) {
	var fields []pbField

	if got := pbUint(fields, 1); got != 0 {
		t.Errorf("pbUint = %d, want 0", got)
	}
	if got := pbBool(fields, 1); got {
		t.Error("pbBool = true, want false")
	}
	if got := pbString(fields, 1); got != "" {
		t.Errorf("pbString = %q, want an empty string", got)
	}
	if _, ok := pbFieldByNumber(fields, 1); ok {
		t.Error("pbFieldByNumber found a field in an empty message")
	}
	if repeated := pbRepeated(fields, 1); len(repeated) != 0 {
		t.Errorf("pbRepeated = %d fields, want 0", len(repeated))
	}
}

// A length-delimited field carries no integer value, and neither does a varint
// carry a string: reading a field as the wrong type yields its zero value
// instead of the raw bytes
func TestPbFieldAccessorsIgnoreWireType(t *testing.T) {
	fields, err := pbParse(pbTestConcat(pbTestString(1, "text"), pbTestVarint(2, 5)))
	if err != nil {
		t.Fatalf("pbParse: %v", err)
	}

	if got := pbUint(fields, 1); got != 0 {
		t.Errorf("pbUint on a length-delimited field = %d, want 0", got)
	}
	if got := pbString(fields, 2); got != "" {
		t.Errorf("pbString on a varint field = %q, want an empty string", got)
	}
	if got := pbBool(fields, 2); !got {
		t.Error("pbBool on a non-zero varint field = false, want true")
	}
}
