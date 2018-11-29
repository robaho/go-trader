package exchange

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/shopspring/decimal"
)

type sessionOrder struct {
	client exchangeClient
	order  *Order
	time   time.Time
}

type quotePair struct {
	bid sessionOrder
	ask sessionOrder
}

func (so sessionOrder) String() string {
	return fmt.Sprint(so.client.SessionID(), so.order)
}

type exchangeClient interface {
	SendOrderStatus(so sessionOrder)
	SendTrades(trades []trade)
	SessionID() string
}

type session struct {
	sync.Mutex
	id     string
	orders map[OrderID]*Order
	quotes map[Instrument]quotePair
	client exchangeClient
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

func (e *exchange) newSession(client exchangeClient) *session {
	s := session{}
	s.id = client.SessionID()
	s.orders = make(map[OrderID]*Order)
	s.quotes = make(map[Instrument]quotePair)
	s.client = client

	e.sessions.Store(client, &s)

	return &s
}

// locking the session is probably not needed, as quickfix ensures a single thread
// processes all of the work for a "fix session"
// still, if it is uncontended it is very cheap
func (e *exchange) lockSession(client exchangeClient) *session {
	s, ok := e.sessions.Load(client)
	if !ok {
		fmt.Println("new session", client)
		s = e.newSession(client)
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

func (e *exchange) CreateOrder(client exchangeClient, order *Order) (OrderID, error) {
	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	nextOrder := atomic.AddInt32(&e.nextOrder, 1)

	s := e.lockSession(client)
	defer s.Unlock()

	var orderID = order.Id

	order.ExchangeId = strconv.Itoa(int(nextOrder))

	s.orders[orderID] = order

	so := sessionOrder{client, order, time.Now()}

	trades, err := ob.add(so)
	if err != nil {
		return -1, err
	}

	book := ob.buildBook()
	sendMarketData(MarketEvent{book, trades})
	client.SendTrades(trades)
	if len(trades) == 0 || order.OrderState == Cancelled {
		client.SendOrderStatus(so)
	}

	return orderID, nil
}

func (e *exchange) ModifyOrder(client exchangeClient, orderId OrderID, price decimal.Decimal, quantity decimal.Decimal) error {
	s := e.lockSession(client)
	defer s.Unlock()

	order, ok := s.orders[orderId]
	if !ok {
		return OrderNotFound
	}

	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	so := sessionOrder{client, order, time.Now()}
	err := ob.remove(so)
	if err != nil {
		client.SendOrderStatus(so)
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
	client.SendTrades(trades)
	if len(trades) == 0 {
		client.SendOrderStatus(so)
	}

	return nil
}

func (e *exchange) CancelOrder(client exchangeClient, orderId OrderID) error {
	s := e.lockSession(client)
	defer s.Unlock()

	order, ok := s.orders[orderId]
	if !ok {
		return OrderNotFound
	}
	ob := e.lockOrderBook(order.Instrument)
	defer ob.Unlock()

	so := sessionOrder{client, order, time.Now()}
	err := ob.remove(so)
	if err != nil {
		return err
	}
	book := ob.buildBook()
	sendMarketData(MarketEvent{book: book})
	client.SendOrderStatus(so)

	return nil
}

func (e *exchange) Quote(client exchangeClient, instrument Instrument, bidPrice decimal.Decimal, bidQuantity decimal.Decimal, askPrice decimal.Decimal, askQuantity decimal.Decimal) error {
	ob := e.lockOrderBook(instrument)
	defer ob.Unlock()

	s := e.lockSession(client)
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
		so := sessionOrder{client, order, time.Now()}
		qp.bid = so
		bidTrades, _ := ob.add(so)
		if bidTrades != nil {
			trades = append(trades, bidTrades...)
		}
	}
	if askPrice != ZERO {
		order := LimitOrder(instrument, Sell, askPrice, askQuantity)
		order.ExchangeId = "quote.ask." + strconv.FormatInt(instrument.ID(), 10)
		so := sessionOrder{client, order, time.Now()}
		qp.ask = so
		askTrades, _ := ob.add(so)
		if askTrades != nil {
			trades = append(trades, askTrades...)
		}
	}
	s.quotes[instrument] = qp

	book := ob.buildBook()
	sendMarketData(MarketEvent{book, trades})

	client.SendTrades(trades)

	return nil
}

var TheExchange exchange

func (e *exchange) ListSessions() string {
	var s []string

	e.sessions.Range(func(key, value interface{}) bool {
		s = append(s, key.(exchangeClient).SessionID())
		return true
	})
	return strings.Join(s, ",")
}
func (e *exchange) SessionDisconnect(client exchangeClient) {
	orderCount := 0
	quoteCount := 0

	s := e.lockSession(client)
	defer s.Unlock()

	for _, v := range s.orders {
		ob := e.lockOrderBook(v.Instrument)
		so := sessionOrder{client: client, order: v}
		ob.remove(so)
		client.SendOrderStatus(so)
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
	fmt.Println("session", client.SessionID(), "disconnected, cancelled", orderCount, "orders", quoteCount, "quotes")
}
func (e *exchange) Start() {
	startMarketData()
}
