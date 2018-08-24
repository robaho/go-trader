package internal

import "sync"
import (
	. "common"
	"fmt"
	"github.com/shopspring/decimal"
	"sort"
	"sync/atomic"
	"time"
)

type orderLevel struct {
	price  decimal.Decimal
	orders []sessionOrder
}

func (ol orderLevel) String() string {
	var s = ol.price.String() + " @ ("
	for i, v := range ol.orders {
		if i > 0 {
			s += ","
		}
		s += v.order.Quantity.String()
	}
	return s + ")"
}

type orderBook struct {
	sync.Mutex
	Instrument Instrument
	bids       []orderLevel
	asks       []orderLevel
}

func (ob *orderBook) String() string {
	return fmt.Sprintf("bids %v, asks %v}", ob.bids, ob.asks)
}

type trade struct {
	buyer    sessionOrder
	seller   sessionOrder
	price    decimal.Decimal
	quantity decimal.Decimal
	tradeid  int64
	when     time.Time

	buyRemaining  decimal.Decimal
	sellRemaining decimal.Decimal
}

func (ob *orderBook) add(so sessionOrder) ([]trade, error) {
	so.order.OrderState = Booked

	if so.order.Side == Buy {
		levels, index := findLevel(ob.bids, so.order.Price, false, true)
		levels[index].orders = append(levels[index].orders, so)
		ob.bids = levels
	} else {
		levels, index := findLevel(ob.asks, so.order.Price, true, true)
		levels[index].orders = append(levels[index].orders, so)
		ob.asks = levels
	}

	// match and build trades
	var trades = matchTrades(ob)

	return trades, nil
}

var nextTradeID int64 = 0

func matchTrades(book *orderBook) []trade {
	var trades []trade
	var tradeID int64 = 0
	var when = time.Now()
	for len(book.bids) > 0 && len(book.asks) > 0 {
		bidL := book.bids[0]
		askL := book.asks[0]

		if !bidL.price.GreaterThanOrEqual(askL.price) {
			break
		}
		var bid = bidL.orders[0]
		var ask = askL.orders[0]

		var price decimal.Decimal
		// need to use price of resting order
		if bid.time.Before(ask.time) {
			price = bid.order.Price
		} else {
			price = ask.order.Price
		}
		var qty = decimal.Min(bid.order.Remaining, ask.order.Remaining)

		var trade = trade{}

		if tradeID == 0 {
			// use same tradeID for all trades
			tradeID = atomic.AddInt64(&nextTradeID, 1)
		}

		trade.price = price
		trade.quantity = qty
		trade.buyer = bid
		trade.seller = ask
		trade.tradeid = tradeID
		trade.when = when

		fill(bid.order, qty, price)
		fill(ask.order, qty, price)

		trade.buyRemaining = bid.order.Remaining
		trade.sellRemaining = ask.order.Remaining

		trades = append(trades, trade)

		if bid.order.Remaining.Equals(ZERO) {
			book.remove(bid)
		}
		if ask.order.Remaining.Equals(ZERO) {
			book.remove(ask)
		}
	}
	return trades
}

func fill(order *Order, qty decimal.Decimal, price decimal.Decimal) {
	order.Remaining = order.Remaining.Sub(qty)
	if order.Remaining.Equals(ZERO) {
		order.OrderState = Filled
	} else {
		order.OrderState = PartialFill
	}
}

func (ob *orderBook) remove(so sessionOrder) error {

	var levels []orderLevel
	var reverse bool

	if so.order.Side == Buy {
		levels = ob.bids
		reverse = false
	} else {
		levels = ob.asks
		reverse = true
	}

	_, index := findLevel(levels, so.order.Price, reverse, false)
	if index == -1 {
		return OrderNotFound
	}
	var newlevel []sessionOrder
	// search and remove sessionOrder
	for _, v := range levels[index].orders {
		if v.order.ExchangeId != so.order.ExchangeId {
			newlevel = append(newlevel, v)
		}
	}
	if len(newlevel) == 0 { // remove empty levels
		levels = append(levels[:index], levels[index+1:]...)
	} else {
		levels[index].orders = newlevel
	}

	if so.order.Side == Buy {
		ob.bids = levels
		reverse = false
	} else {
		ob.asks = levels
	}

	if so.order.IsActive() {
		so.order.OrderState = Cancelled
	}

	return nil
}

func (ob *orderBook) buildBook() *Book {
	var book = new(Book)

	book.Instrument = ob.Instrument
	book.Bids = createBookLevels(ob.bids)
	book.Asks = createBookLevels(ob.asks)

	return book
}

func createBookLevels(obLevels []orderLevel) (levels []BookLevel) {
	for _, i := range obLevels {
		var qty = ZERO
		for _, j := range i.orders {
			qty = qty.Add(j.order.Remaining)
		}
		var level = BookLevel{i.price, qty}
		levels = append(levels, level)
	}
	return levels
}

func findLevel(data []orderLevel, price decimal.Decimal, reverse bool, insert bool) ([]orderLevel, int) {
	f := func(i int) bool {
		if reverse {
			return data[i].price.GreaterThanOrEqual(price)
		} else {
			return data[i].price.LessThanOrEqual(price)
		}
	}
	index := sort.Search(len(data), f)
	if index < len(data) && data[index].price.Equals(price) {
		return data, index
	}
	// level was not found...
	if !insert {
		return data, -1
	}
	ol := orderLevel{}
	ol.price = price
	data = append(data, ol)
	copy(data[index+1:], data[index:])
	data[index] = ol
	return data, index
}
