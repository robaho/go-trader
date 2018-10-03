package exchange

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quickfixgo/enum"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/shopspring/decimal"
)

type quotePair struct {
	bid sessionOrder
	ask sessionOrder
}

type sessionOrder struct {
	session string
	order   *Order
	time    time.Time
}

func (so sessionOrder) String() string {
	return fmt.Sprint(so.session, so.order)
}

type session struct {
	sync.Mutex
	id     string
	orders map[OrderID]*Order
	quotes map[Instrument]quotePair
}

var buyMarketPrice = NewDecimal("9999999999999")
var sellMarketPrice = ZERO

// return the "effective price" of an order - so market orders can always be at the top
func (so *sessionOrder) getPrice() decimal.Decimal {
	if so.order.OrderType == Market {
		if so.order.Side == Buy {
			return buyMarketPrice
		} else {
			return sellMarketPrice
		}
	}
	return so.order.Price
}

func newSession(id string) session {
	s := session{}
	s.id = id
	s.orders = make(map[OrderID]*Order)
	s.quotes = make(map[Instrument]quotePair)
	return s
}

// locking the session is probably not needed, as quickfix ensures a single thread
// processes all of the work for a "fix session"
// still, if it is uncontended it is very cheap
func (e *exchange) lockSession(id string) *session {
	s, ok := e.sessions.Load(id)
	if !ok {
		fmt.Println("warning! unknown session", id)
		s = newSession(id)
	}
	s.(*session).Lock()
	return s.(*session)
}

func (e *exchange) lockOrderBook(instrument Instrument) *orderBook {
	ob, ok := e.orderBooks.Load(instrument)
	if !ok {
		ob = &orderBook{Instrument: instrument}
		ob, _ = e.orderBooks.LoadOrStore(instrument, ob)
	}
	_ob := ob.(*orderBook)
	_ob.Lock()
	return _ob
}

type exchange struct {
	connected  bool
	callbacks  []ConnectorCallback
	orderBooks sync.Map // map of Instrument to *orderBook
	sessions   sync.Map // map of string to session
	nextOrder  int32
}

func (e *exchange) CreateOrder(session string, order *Order) (OrderID, error) {
	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	nextOrder := atomic.AddInt32(&e.nextOrder, 1)

	s := e.lockSession(session)
	defer s.Unlock()

	var orderID = order.Id

	order.ExchangeId = strconv.Itoa(int(nextOrder))

	s.orders[orderID] = order

	so := sessionOrder{session, order, time.Now()}

	trades, err := ob.add(so)
	if err != nil {
		return -1, err
	}

	book := ob.buildBook()
	sendMarketData(MarketEvent{book, trades})
	App.sendExecutionReports(trades)
	if len(trades) == 0 || order.OrderState == Cancelled {
		App.sendExecutionReport(enum.ExecType_NEW, so)
	}

	return orderID, nil
}

func (e *exchange) ModifyOrder(session string, orderId OrderID, price decimal.Decimal, quantity decimal.Decimal) error {
	s := e.lockSession(session)
	defer s.Unlock()

	order, ok := s.orders[orderId]
	if !ok {
		return OrderNotFound
	}

	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	so := sessionOrder{session, order, time.Now()}
	err := ob.remove(so)
	if err != nil {
		App.sendExecutionReport(enum.ExecType_REJECTED, so)
		return nil
	}

	order.Price = price
	order.Quantity = quantity
	order.Remaining = quantity

	trades, err := ob.add(so)
	if err != nil {
		return nil
	}
	book := ob.buildBook()
	sendMarketData(MarketEvent{book, trades})
	App.sendExecutionReports(trades)
	if len(trades) == 0 {
		App.sendExecutionReport(enum.ExecType_REPLACED, so)
	}

	return nil
}

func (e *exchange) CancelOrder(session string, orderId OrderID) error {
	s := e.lockSession(session)
	defer s.Unlock()

	order, ok := s.orders[orderId]
	if !ok {
		return OrderNotFound
	}
	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	so := sessionOrder{session, order, time.Now()}
	err := ob.remove(so)
	if err != nil {
		return err
	}
	book := ob.buildBook()
	sendMarketData(MarketEvent{book: book})
	App.sendExecutionReport(enum.ExecType_CANCELED, so)

	return nil
}

func (e *exchange) Quote(session string, instrument Instrument, bidPrice decimal.Decimal, bidQuantity decimal.Decimal, askPrice decimal.Decimal, askQuantity decimal.Decimal) error {
	ob := e.lockOrderBook(instrument)
	defer ob.Unlock()

	s := e.lockSession(session)
	defer s.Unlock()

	qp, ok := s.quotes[instrument]
	if ok {
		if qp.bid.order != nil {
			ob.remove(qp.bid)
			qp.bid.order = nil
		}
		if qp.ask.order != nil {
			ob.remove(qp.ask)
			qp.ask.order = nil
		}
	} else {
		qp = quotePair{}
	}
	var trades []trade
	if bidPrice != ZERO {
		order := LimitOrder(instrument, Buy, bidPrice, bidQuantity)
		order.ExchangeId = "quote.bid." + strconv.FormatInt(instrument.ID(), 10)
		so := sessionOrder{session, order, time.Now()}
		qp.bid = so
		bidTrades, _ := ob.add(so)
		if bidTrades != nil {
			trades = append(trades, bidTrades...)
		}
	}
	if askPrice != ZERO {
		order := LimitOrder(instrument, Sell, askPrice, askQuantity)
		order.ExchangeId = "quote.ask." + strconv.FormatInt(instrument.ID(), 10)
		so := sessionOrder{session, order, time.Now()}
		qp.ask = so
		askTrades, _ := ob.add(so)
		if askTrades != nil {
			trades = append(trades, askTrades...)
		}
	}
	s.quotes[instrument] = qp

	book := ob.buildBook()
	sendMarketData(MarketEvent{book, trades})

	App.sendExecutionReports(trades)

	return nil
}

var TheExchange exchange

func (e *exchange) ListSessions() string {
	var s []string

	e.sessions.Range(func(key, value interface{}) bool {
		s = append(s, key.(string))
		return true
	})
	return strings.Join(s, ",")
}
func (e *exchange) SessionDisconnect(session string) {
	orderCount := 0
	quoteCount := 0

	s := e.lockSession(session)
	for _, v := range s.orders {
		ob := e.lockOrderBook(v.Instrument)
		so := sessionOrder{session: session, order: v}
		ob.remove(so)
		App.sendExecutionReport(enum.ExecType_CANCELED, so)
		sendMarketData(MarketEvent{book: ob.buildBook()})
		ob.Unlock()
		orderCount++
	}
	for k, v := range s.quotes {
		ob := e.lockOrderBook(k)
		ob.remove(v.bid)
		ob.remove(v.ask)
		sendMarketData(MarketEvent{book: ob.buildBook()})
		ob.Unlock()
		quoteCount++
	}
	fmt.Println("session", session, "disconnected, cancelled", orderCount, "orders", quoteCount, "quotes")
}
func (e *exchange) Start() {
	startMarketData()
}
