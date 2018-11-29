package common

import (
	"errors"
	"github.com/shopspring/decimal"
	"io"
	"strconv"
	"time"
)

var ZERO = decimal.NewFromFloat(0.0)

func NewDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}
func ParseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func ToFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}

var overflow = errors.New("binary: varint overflows a 64-bit integer")

// ReadUvarint reads an encoded unsigned integer from r and returns it as a uint64.
func ReadUvarint(r io.ByteReader) (uint64, error) {
	var x uint64
	var s uint
	for i := 0; ; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return x, err
		}
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				return x, overflow
			}
			return x | uint64(b)<<s, nil
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
}

// ReadVarint reads an encoded signed integer from r and returns it as an int64.
func ReadVarint(r io.ByteReader) (int64, error) {
	ux, err := ReadUvarint(r) // ok to continue in presence of error
	x := int64(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x, err
}

// PutUvarint encodes a uint64 into buf and returns the number of bytes written.
func PutUvarint(w io.ByteWriter, x uint64) int {
	i := 0
	for x >= 0x80 {
		w.WriteByte(byte(x) | 0x80)
		x >>= 7
		i++
	}
	w.WriteByte(byte(x))
	return i + 1
}

// PutVarint encodes an int64 into buf and returns the number of bytes written.
func PutVarint(w io.ByteWriter, x int64) int {
	ux := uint64(x) << 1
	if x < 0 {
		ux = ^ux
	}
	return PutUvarint(w, ux)
}

func EncodeDecimal(w io.ByteWriter, d decimal.Decimal) {
	var exp int64 = int64(d.Exponent())
	var coef int64 = d.Coefficient().Int64()
	PutVarint(w, exp)
	PutVarint(w, coef)
}

func DecodeDecimal(r io.ByteReader) decimal.Decimal {
	exp, _ := ReadVarint(r)
	coef, _ := ReadVarint(r)

	return decimal.New(coef, int32(exp))
}

func EncodeString(w io.ByteWriter, s string) {
	bytes := []byte(s)
	w.WriteByte(byte(len(bytes)))
	for i := 0; i < len(bytes); i++ {
		w.WriteByte(bytes[i])
	}
}
func DecodeString(r io.ByteReader) string {
	len, _ := r.ReadByte()
	bytes := make([]byte, 0)
	for i := 0; i < int(len); i++ {
		b, _ := r.ReadByte()
		bytes = append(bytes, b)
	}
	return string(bytes)
}
func EncodeTime(w io.ByteWriter, time time.Time) {
	PutVarint(w, time.UnixNano())
}
func DecodeTime(r io.ByteReader) time.Time {
	ns, _ := ReadVarint(r)
	return time.Unix(0, ns)
}
func CmpTime(t1 time.Time, t2 time.Time) int {
	if t1.Equal(t2) {
		return 0
	} else if t1.Before(t2) {
		return -1
	} else {
		return 1
	}
}
