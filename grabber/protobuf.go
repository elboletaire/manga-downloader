// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Minimal protobuf wire-format reader.
//
// MangaPlus's app API answers with protobuf (there is no JSON alongside it),
// and only three of its messages are needed, for a handful of fields each.
// Pulling in the protobuf runtime and generating the whole schema for that
// would be a lot of machinery for reading a few numbers, so this only
// understands the wire format itself: a message decodes to a flat list of
// fields, out of which the caller picks the ones it wants by number and
// ignores the rest (which is what protobuf's compatibility rules are about).
//
// Everything here stays unexported: it's an implementation detail of the
// MangaPlus grabber, not a general purpose protobuf library.

// The protobuf wire types. Frames 3 and 4 (the grouped encodings) were dropped
// in proto3 and are rejected rather than guessed at: there's no way to know
// where a group ends without the schema.
const (
	pbWireVarint  = 0 // varint: int32, int64, uint32, uint64, sint32, sint64, bool, enum
	pbWireFixed64 = 1 // fixed64: fixed64, sfixed64, double
	pbWireBytes   = 2 // length-delimited: string, bytes, embedded message, packed repeated
	pbWireFixed32 = 5 // fixed32: fixed32, sfixed32, float
)

// errPbTruncated is returned whenever the input ends in the middle of a field
// (or of a field's payload): a partial message must never look like a
// successfully decoded, shorter one.
var errPbTruncated = errors.New("truncated protobuf message")

// pbField is a single field of a protobuf message, still in wire format.
// Exactly one of u and b carries the value, depending on wire.
type pbField struct {
	// num is the field number, as declared in the schema
	num int
	// wire is the wire type, which says how the value is stored
	wire int
	// u holds the value of a varint or a fixed-width field
	u uint64
	// b holds the raw payload of a length-delimited field, undecoded
	b []byte
}

// Uint returns the field as an unsigned integer. Only the integer wire types
// (varints, which is how protobuf encodes integers, booleans and enums, plus
// the fixed-width ones) have one; a length-delimited field reads as 0.
func (f pbField) Uint() uint64 {
	switch f.wire {
	case pbWireVarint, pbWireFixed64, pbWireFixed32:
		return f.u
	}
	return 0
}

// String returns the field as a string. Only length-delimited fields (which is
// how protobuf encodes strings, bytes and embedded messages) have one.
func (f pbField) String() string {
	if f.wire == pbWireBytes {
		return string(f.b)
	}
	return ""
}

// pbReadVarint reads a base-128 varint, returning its value and how many bytes
// it took. A varint that never ends within the input is truncated; one that
// runs past 10 bytes can't fit in a uint64, so it's malformed rather than
// merely incomplete.
func pbReadVarint(data []byte) (uint64, int, error) {
	var value uint64
	for i := 0; i < len(data); i++ {
		if i == 10 {
			return 0, 0, errors.New("protobuf: varint longer than 10 bytes")
		}
		b := data[i]
		value |= uint64(b&0x7f) << (7 * uint(i))
		if b&0x80 == 0 {
			return value, i + 1, nil
		}
	}

	return 0, 0, errPbTruncated
}

// pbParse decodes a message into its flat list of fields. Embedded messages
// stay as raw bytes (parse them with pbParse or pbMessage), values keep their
// wire format (use Uint/String to read them) and fields of any number are
// returned, so the caller decides which ones matter.
func pbParse(data []byte) ([]pbField, error) {
	var fields []pbField

	for i := 0; i < len(data); {
		key, size, err := pbReadVarint(data[i:])
		if err != nil {
			return nil, err
		}
		i += size

		field := pbField{num: int(key >> 3), wire: int(key & 0x7)}
		if field.num == 0 {
			return nil, errors.New("protobuf: field number 0 is invalid")
		}

		switch field.wire {
		case pbWireVarint:
			field.u, size, err = pbReadVarint(data[i:])
		case pbWireFixed64:
			if len(data)-i < 8 {
				return nil, errPbTruncated
			}
			field.u, size = binary.LittleEndian.Uint64(data[i:]), 8
		case pbWireBytes:
			var length uint64
			length, size, err = pbReadVarint(data[i:])
			if err == nil {
				if length > uint64(len(data)-i-size) {
					return nil, errPbTruncated
				}
				field.b = data[i+size : i+size+int(length)]
				size += int(length)
			}
		case pbWireFixed32:
			if len(data)-i < 4 {
				return nil, errPbTruncated
			}
			field.u, size = uint64(binary.LittleEndian.Uint32(data[i:])), 4
		default:
			return nil, fmt.Errorf("protobuf: unsupported wire type %d", field.wire)
		}
		if err != nil {
			return nil, err
		}
		i += size

		fields = append(fields, field)
	}

	return fields, nil
}

// pbMessage parses a length-delimited field as an embedded message. A field of
// any other wire type can't hold one.
func pbMessage(f pbField) ([]pbField, error) {
	if f.wire != pbWireBytes {
		return nil, fmt.Errorf("protobuf: field %d is not a message (wire type %d)", f.num, f.wire)
	}

	return pbParse(f.b)
}

// pbRepeated returns every field with the given number, in the order they
// appear: repeated fields and non-repeated ones that were set more than once
// alike (for the latter, the last one is the one that counts, see
// pbFieldByNumber).
func pbRepeated(fields []pbField, num int) []pbField {
	var matches []pbField

	for _, f := range fields {
		if f.num == num {
			matches = append(matches, f)
		}
	}

	return matches
}

// pbFieldByNumber returns the last field with the given number, which is the
// one that counts when a singular field was set more than once.
func pbFieldByNumber(fields []pbField, num int) (pbField, bool) {
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i].num == num {
			return fields[i], true
		}
	}

	return pbField{}, false
}

// pbUint returns the value of the given field as an unsigned integer, or 0
// when it's absent (which is what protobuf means by an unset scalar: its type's
// default value, e.g. the first member of an enum).
func pbUint(fields []pbField, num int) uint64 {
	field, ok := pbFieldByNumber(fields, num)
	if !ok {
		return 0
	}
	return field.Uint()
}

// pbBool returns the given field as a boolean, or false when it's absent
func pbBool(fields []pbField, num int) bool {
	return pbUint(fields, num) != 0
}

// pbString returns the value of the given field as a string, or "" when it's
// absent
func pbString(fields []pbField, num int) string {
	field, ok := pbFieldByNumber(fields, num)
	if !ok {
		return ""
	}
	return field.String()
}
