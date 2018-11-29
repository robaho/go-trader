package common

import (
	. "github.com/robaho/fixed"
	"reflect"
)

type BookLevel struct {
	Price    Fixed
	Quantity Fixed
}

type Book struct {
	Instrument Instrument
	Bids       []BookLevel
	Asks       []BookLevel
	Sequence   uint64
}

func (book *Book) String() string {

	var s = "book:"
	if book.Instrument != nil {
		s += book.Instrument.Symbol()
	} else {
		s += "<nil>"
	}
	s = s + " bids: " + toString(book.Bids) + " asks: " + toString(book.Asks)
	return s
}
func (book *Book) Equals(other Book) bool {
	return reflect.DeepEqual(*book, other)

}
func (book *Book) HasBids() bool {
	return len(book.Bids) > 0
}
func (book *Book) HasAsks() bool {
	return len(book.Asks) > 0
}
func (book *Book) IsEmpty() bool {
	return !book.HasBids() && !book.HasAsks()
}
func toString(levels []BookLevel) string {
	var s string
	for i, e := range levels {
		if i > 0 {
			s += ","
		}
		s = s + e.Quantity.String() + " @ " + e.Price.String()
	}
	return s
}
