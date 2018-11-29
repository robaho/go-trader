package common

import (
	"github.com/quickfixgo/enum"
	. "github.com/robaho/fixed"
	"github.com/shopspring/decimal"
)

func ToFixed(d decimal.Decimal) Fixed {
	f, _ := d.Float64()
	return NewDecimalF(f)
}
func ToDecimal(f Fixed) decimal.Decimal {
	return decimal.NewFromFloat(f.Float())
}

func MapToFixSide(side Side) enum.Side {
	switch side {
	case Buy:
		return enum.Side_BUY
	case Sell:
		return enum.Side_SELL
	}
	panic("unsupported side " + side)
}

func MapFromFixSide(side enum.Side) Side {
	switch side {
	case enum.Side_BUY:
		return Buy
	case enum.Side_SELL:
		return Sell
	}
	panic("unsupported side " + side)
}

func MapToFixOrdStatus(state OrderState) enum.OrdStatus {
	switch state {
	case Booked:
		return enum.OrdStatus_NEW
	case PartialFill:
		return enum.OrdStatus_PARTIALLY_FILLED
	case Filled:
		return enum.OrdStatus_FILLED
	case Cancelled:
		return enum.OrdStatus_CANCELED
	case Rejected:
		return enum.OrdStatus_REJECTED
	}
	panic("unknown OrderState " + state)
}

func MapFromFixOrdStatus(ordStatus enum.OrdStatus) OrderState {
	switch ordStatus {
	case enum.OrdStatus_NEW:
		return Booked
	case enum.OrdStatus_CANCELED:
		return Cancelled
	case enum.OrdStatus_PARTIALLY_FILLED:
		return PartialFill
	case enum.OrdStatus_FILLED:
		return Filled
	case enum.OrdStatus_REJECTED:
		return Rejected
	}
	panic("unsupported order status " + ordStatus)
}
