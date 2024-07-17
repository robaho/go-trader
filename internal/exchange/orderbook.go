package exchange

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/robaho/fixed"

	. "github.com/robaho/go-trader/pkg/common"
)

type priceLevel struct {
	orderList
	price Fixed
}

func (level priceLevel) String() string {
	return fmt.Sprint("", level.price, "=", &level.orderList)
}

type orderBook struct {
	sync.Mutex
	Instrument
	bids []priceLevel
	asks []priceLevel
}

type trade struct {
	buyer    sessionOrder
	seller   sessionOrder
	price    Fixed
	quantity Fixed
	tradeid  int64
	when     time.Time

	buyRemaining  Fixed
	sellRemaining Fixed
}

func (ob *orderBook) String() string {
	return fmt.Sprint("bids:", ob.bids, ", asks:", ob.asks)
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

	// cancel any remaining market order
	if so.order.OrderType == Market && so.order.IsActive() {
		so.order.OrderState = Cancelled
		ob.remove(so)
	}

	return trades, nil
}

func insertSort(levels []priceLevel, so sessionOrder, direction int) []priceLevel {
	index := sort.Search(len(levels), func(i int) bool {
		cmp := so.getPrice().Cmp(levels[i].price) * direction
		return cmp >= 0
	})

	if(index<len(levels) && levels[index].price==so.getPrice()) {
		level := &levels[index];
		level.orderList.pushBack(so)
	} else {
		// add new level
		level := priceLevel{OrderList(),so.getPrice()}
		level.orderList.pushBack(so)
		levels = append(levels[:index], append([]priceLevel{level}, levels[index:]...)...)
	}

	return levels;
}

var nextTradeID int64 = 0

func matchTrades(book *orderBook) []trade {
	var trades []trade
	var tradeID int64 = 0
	var when = time.Now()

	for len(book.bids) > 0 && len(book.asks) > 0 {
		bid := book.bids[0].orderList.top()
		ask := book.asks[0].orderList.top()

		if !bid.getPrice().GreaterThanOrEqual(ask.getPrice()) {
			break
		}

		var price Fixed
		// need to use price of resting order
		if bid.time.Before(ask.time) {
			price = bid.order.Price
		} else {
			price = ask.order.Price
		}

		var qty = MinDecimal(bid.order.Remaining, ask.order.Remaining)

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

		if bid.order.Remaining.Equal(ZERO) {
			book.remove(bid)
		}
		if ask.order.Remaining.Equal(ZERO) {
			book.remove(ask)
		}
	}
	return trades
}

func fill(order *Order, qty Fixed, price Fixed) {
	order.Remaining = order.Remaining.Sub(qty)
	if order.Remaining.Equal(ZERO) {
		order.OrderState = Filled
	} else {
		order.OrderState = PartialFill
	}
}

func (ob *orderBook) remove(so sessionOrder) error {

	var levels []priceLevel
	var direction int

	if so.order.Side == Buy {
		levels = ob.bids
		direction = 1
	} else {
		levels = ob.asks
		direction = -1
	}

	index := sort.Search(len(levels), func(i int) bool {
		cmp := so.getPrice().Cmp(levels[i].price) * direction
		return cmp >= 0
	})

	if index>=len(levels) || levels[index].price.Cmp(so.getPrice())!=0 {
		return OrderNotFound
	}

	level := &levels[index]
	err := level.orderList.remove(so)

	if err!=nil {
		return err
	}

	if level.orderList.size==0 {
		levels = append(levels[:index],levels[index+1:]...)
	}
	
	if so.order.Side == Buy {
		ob.bids = levels
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

func createBookLevels(_levels []priceLevel) []BookLevel {
	var levels []BookLevel

	if len(_levels) == 0 {
		return levels
	}
	for _, level := range _levels {
		quantity := ZERO
		for node := level.head; node!=nil; node = node.next {
			quantity = quantity.Add(node.order.order.Remaining)
		}
		bl := BookLevel{Price: level.price, Quantity: quantity}
		levels = append(levels, bl)
	}
	return levels
}
