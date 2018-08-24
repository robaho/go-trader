package common

import "time"
import (
	"github.com/shopspring/decimal"
	"reflect"
)

type Expiration struct {
	time.Time
}

type Instrument interface {
	ID() int64
	Symbol() string
	Group() string
}

type base struct {
	id     int64
	symbol string
	group  string
}

type Equity struct {
	base
}

func (e *Equity) String() string {
	return "equity:" + e.symbol
}

func NewEquity(id int64, symbol string) Instrument {
	e := Equity{base{id, symbol, symbol}}
	return e
}

func (b base) ID() int64 {
	return b.id
}
func (b base) Symbol() string {
	return b.symbol
}
func (b base) Group() string {
	return b.group
}
func (b base) String() string {
	return b.symbol
}

type optionType string

const (
	Call optionType = "call"
	Put  optionType = "put"
)

type Option struct {
	Instrument
	Underlying Instrument
	Expires    Expiration
	Strike     decimal.Decimal
	OptionTYpe optionType
}

type OptionLeg struct {
	Option *Option
	Ratio  int
}

type OptionStrategy struct {
	Instrument
	Expires Expiration
	Legs    []OptionLeg
}

type BookLevel struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
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
