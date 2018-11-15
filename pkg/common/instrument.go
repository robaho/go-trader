package common

import "time"
import (
	"github.com/shopspring/decimal"
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

type instrumentImpl struct {
	base
}

func (e *instrumentImpl) String() string {
	return e.symbol
}

func NewInstrument(id int64, symbol string) Instrument {
	e := instrumentImpl{base{id, symbol, symbol}}
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
