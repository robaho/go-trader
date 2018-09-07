package exchange

import "sync"
import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/shopspring/decimal"
)

type orderBook struct {
	sync.Mutex
	Instrument
	bids []sessionOrder
	asks []sessionOrder
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

func (ob *orderBook) String() string {
	return fmt.Sprint("bids:", ob.bids, "asks:", ob.asks)
}

func (ob *orderBook) add(so sessionOrder) ([]trade, error) {
	so.order.OrderState = Booked

	if so.order.Side == Buy {
		ob.bids = insertSort(ob.bids, so, 1)
	} else {
		ob.asks = insertSort(ob.asks, so, -1)
	}

	// match and build trades
	var trades = matchTrades(ob)

	return trades, nil
}

func insertSort(orders []sessionOrder, so sessionOrder, direction int) []sessionOrder {
	index := sort.Search(len(orders), func(i int) bool {
		cmp := so.order.Price.Cmp(orders[i].order.Price) * direction
		if cmp == 0 {
			cmp = CmpTime(so.time, orders[i].time)
		}
		return cmp >= 0
	})

	return append(orders[:index], append([]sessionOrder{so}, orders[index:]...)...)
}

var nextTradeID int64 = 0

func matchTrades(book *orderBook) []trade {
	var trades []trade
	var tradeID int64 = 0
	var when = time.Now()

	for len(book.bids) > 0 && len(book.asks) > 0 {
		bid := book.bids[0]
		ask := book.asks[0]

		if !bid.order.Price.GreaterThanOrEqual(ask.order.Price) {
			break
		}

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

	var removed bool

	removeFN := func(orders *[]sessionOrder, so sessionOrder) bool {
		for i, v := range *orders {
			if v.order == so.order {
				*orders = append((*orders)[:i], (*orders)[i+1:]...)
				return true
			}
		}
		return false
	}

	if so.order.Side == Buy {
		removed = removeFN(&ob.bids, so)
	} else {
		removed = removeFN(&ob.asks, so)
	}

	if !removed {
		return OrderNotFound
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

func createBookLevels(orders []sessionOrder) []BookLevel {
	var levels []BookLevel

	if len(orders) == 0 {
		return levels
	}

	price := orders[0].order.Price
	quantity := decimal.Zero

	for _, v := range orders {
		if v.order.Price.Equals(price) {
			quantity = quantity.Add(v.order.Remaining)
		} else {
			bl := BookLevel{Price: price, Quantity: quantity}
			levels = append(levels, bl)
			price = v.order.Price
			quantity = v.order.Remaining
		}
	}
	bl := BookLevel{Price: price, Quantity: quantity}
	levels = append(levels, bl)
	return levels
}
