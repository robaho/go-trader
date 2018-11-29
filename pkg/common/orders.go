package common

import (
	. "github.com/robaho/fixed"
	"strconv"
	"sync"
)

type Side string
type OrderState string
type OrderType string

type OrderID int32

func (id OrderID) String() string {
	return strconv.Itoa(int(id))
}
func NewOrderID(id string) OrderID {
	return OrderID(ParseInt(id))
}

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

const (
	Market OrderType = "market"
	Limit  OrderType = "limit"
)

const (
	New         OrderState = "new"
	Booked      OrderState = "booked"
	PartialFill OrderState = "partial"
	Filled      OrderState = "filled"
	Cancelled   OrderState = "cancelled"
	Rejected    OrderState = "rejected"
)

type Order struct {
	sync.RWMutex
	Instrument
	Id         OrderID
	ExchangeId string
	Price      Fixed
	Side
	Quantity  Fixed
	Remaining Fixed
	OrderType
	OrderState
	RejectReason string
}

func (order *Order) String() string {
	return "oid " + order.Id.String() +
		" eoid " + order.ExchangeId +
		" " + order.Instrument.Symbol() +
		" " + order.Quantity.String() + "@" + order.Price.String() +
		" remaining " + order.Remaining.String() +
		" " + string(order.OrderState)
}
func (order *Order) IsActive() bool {
	return order.OrderState != Filled && order.OrderState != Cancelled && order.OrderState != Rejected
}

func MarketOrder(instrument Instrument, side Side, quantity Fixed) *Order {
	order := newOrder(instrument, side, quantity)
	order.Price = ZERO
	order.OrderType = Market
	return order
}

func LimitOrder(instrument Instrument, side Side, price Fixed, quantity Fixed) *Order {
	order := newOrder(instrument, side, quantity)
	order.Price = price
	order.OrderType = Limit
	return order
}
func newOrder(instrument Instrument, side Side, qty Fixed) *Order {
	order := new(Order)
	order.Instrument = instrument
	order.Side = side
	order.Quantity = qty
	order.Remaining = qty
	order.OrderState = New
	return order
}
