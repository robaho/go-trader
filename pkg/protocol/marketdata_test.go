package protocol

import "testing"
import (
	"bytes"
	. "github.com/robaho/go-trader/pkg/common"
	"reflect"
)

func TestEncodeDecodeBook(t *testing.T) {

	instrument := NewEquity(12345, "IBM")
	IMap.Put(instrument)

	book := Book{Instrument: instrument, Sequence: 123456789}
	book.Bids = []BookLevel{BookLevel{NewDecimal("99.4567"), NewDecimal("100")}}
	book.Asks = []BookLevel{BookLevel{NewDecimal("100.4567"), NewDecimal("120")}}

	buf := new(bytes.Buffer)
	buf.Write(encodeBook(&book))

	book2 := decodeBook(buf, instrument)

	if !reflect.DeepEqual(book, *book2) {
		t.Error("books do not match", &book, book2)
	}
}
