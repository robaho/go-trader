package common

import (
	"errors"
	"github.com/shopspring/decimal"
	"time"
)

type ExchangeConnector interface {
	IsConnected() bool
	Connect() error
	Disconnect() error

	CreateOrder(order *Order) (OrderID, error)
	ModifyOrder(order OrderID, price decimal.Decimal, quantity decimal.Decimal) error
	CancelOrder(order OrderID) error

	Quote(instrument Instrument, bidPrice decimal.Decimal, bidQuantity decimal.Decimal, askPrice decimal.Decimal, askQuantity decimal.Decimal) error

	GetExchangeCode() string

	// ask exchange to create the instrument if it does not already exist, and assign a numeric instrument id
	// the instruments are not persisted across exchange restarts
	CreateInstrument(symbol string)
	// ask exchange for configured instruments, will be emitted via onInstrument() on the callback. this call
	// blocks until all instruments are received
	DownloadInstruments() error
}

// a fill on an order or quote
type Fill struct {
	Instrument Instrument
	IsQuote    bool
	// Order will be nil on quote trade, the order is unlocked
	Order      *Order
	ExchangeID string
	Quantity   decimal.Decimal
	Price      decimal.Decimal
	Side       Side
	IsLegTrade bool
}

// an exchange trade, not necessarily initiated by the current client
type Trade struct {
	Instrument Instrument
	Quantity   decimal.Decimal
	Price      decimal.Decimal
	ExchangeID string
	TradeTime  time.Time
}

type ConnectorCallback interface {
	OnBook(*Book)
	// the following is for intra-day instrument addition, or initial startup
	OnInstrument(Instrument)
	// the callback will have the order locked, and will unlock when the callback returns
	OnOrderStatus(*Order)
	OnFill(*Fill)
	OnTrade(*Trade)
}

var AlreadyConnected = errors.New("already connected")
var NotConnected = errors.New("not connected")
var ConnectionFailed = errors.New("connection failed")
var OrderNotFound = errors.New("order not found")
var InvalidConnector = errors.New("invalid connector")
var UnknownInstrument = errors.New("unknown instrument")
var UnsupportedOrderType = errors.New("unsupported order type")
var DownloadFailed = errors.New("download failed")
