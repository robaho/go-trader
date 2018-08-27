package common

import (
	"bytes"
	"testing"
	"time"
)

func TestEncodeDecodeDecimal(t *testing.T) {
	d := NewDecimal("12345.6789")

	buf := new(bytes.Buffer)

	EncodeDecimal(buf, d)

	d2 := DecodeDecimal(buf)

	if !d.Equal(d2) {
		t.Error(d, d2)
	}
}

func TestEncodeDecodeString(t *testing.T) {
	s := "this is my sample string"

	buf := new(bytes.Buffer)

	EncodeString(buf, s)

	s2 := DecodeString(buf)

	if s != s2 {
		t.Error(s, s2)
	}
}

func TestEncodeDecodeTime(t *testing.T) {
	ts := time.Now()

	buf := new(bytes.Buffer)

	EncodeTime(buf, ts)

	ts2 := DecodeTime(buf)

	if ts.UnixNano() != ts2.UnixNano() {
		t.Error(ts, ts2)
	}
}
