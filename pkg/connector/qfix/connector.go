package qfix

import (
	"github.com/quickfixgo/fix44/securitydefinitionrequest"
	"github.com/quickfixgo/fix44/securitylistrequest"
	. "github.com/robaho/fixed"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/fix44/massquote"
	"github.com/quickfixgo/fix44/newordersingle"
	"github.com/quickfixgo/fix44/ordercancelreplacerequest"
	"github.com/quickfixgo/fix44/ordercancelrequest"
	"github.com/quickfixgo/quickfix"
	. "github.com/robaho/go-trader/pkg/common"
)

type qfixConnector struct {
	connected bool
	callback  ConnectorCallback
	nextOrder int64
	nextQuote int64
	// holds OrderID->*Order, concurrent since notifications/updates may arrive while order is being processed
	orders    sync.Map
	sessionID quickfix.SessionID
	initiator *quickfix.Initiator
	loggedIn  StatusBool
	// true after all instruments are downloaded from exchange
	downloaded StatusBool
	settings   string
	log        io.Writer
	secReqId   int64
}

func (c *qfixConnector) IsConnected() bool {
	return c.connected
}

func (c *qfixConnector) Connect() error {
	if c.connected {
		return AlreadyConnected
	}

	cfg, err := os.Open(c.settings)
	if err != nil {
		panic(err)
	}
	appSettings, err := quickfix.ParseSettings(cfg)
	if err != nil {
		panic(err)
	}
	storeFactory := quickfix.NewMemoryStoreFactory()
	//logFactory, _ := quickfix.NewFileLogFactory(appSettings)

	useLogging, err := appSettings.GlobalSettings().BoolSetting("Logging")
	var logFactory quickfix.LogFactory
	if useLogging {
		logFactory = quickfix.NewScreenLogFactory()
	} else {
		logFactory = quickfix.NewNullLogFactory()
	}
	initiator, err := quickfix.NewInitiator(newApplication(c), storeFactory, appSettings, logFactory)
	if err != nil {
		panic(err)
	}

	c.initiator = initiator
	c.sessionID = getSession(appSettings.SessionSettings())

	err = initiator.Start()
	if err != nil {
		return err
	}
	// wait for login up to 30 seconds
	if !c.loggedIn.WaitForTrue(30 * 1000) {
		return ConnectionFailed
	}

	c.connected = true

	return nil
}
func getSession(settings map[quickfix.SessionID]*quickfix.SessionSettings) quickfix.SessionID {
	if len(settings) > 1 {
		panic("only a single fix session is supported")
	}
	for k := range settings {
		return k
	}
	panic("no session found")
}

func (c *qfixConnector) Disconnect() error {
	if !c.connected {
		return NotConnected
	}
	c.initiator.Stop()
	c.connected = false
	return nil
}

func (c *qfixConnector) CreateInstrument(symbol string) {
	_reqid := atomic.AddInt64(&c.secReqId, 1)
	reqid := field.NewSecurityReqID(strconv.FormatInt(_reqid, 10))
	reqtype := field.NewSecurityRequestType(enum.SecurityRequestType_SYMBOL)

	msg := securitydefinitionrequest.New(reqid, reqtype)
	msg.SetSymbol(symbol)

	quickfix.SendToTarget(msg, c.sessionID)
}

func (c *qfixConnector) DownloadInstruments() error {
	c.downloaded.SetFalse()

	_reqid := atomic.AddInt64(&c.secReqId, 1)
	reqid := field.NewSecurityReqID(strconv.FormatInt(_reqid, 10))
	reqtype := field.NewSecurityListRequestType(enum.SecurityListRequestType_ALL_SECURITIES)

	msg := securitylistrequest.New(reqid, reqtype)

	err := quickfix.SendToTarget(msg, c.sessionID)
	if err != nil {
		return err
	}

	// wait for login up to 30 seconds
	if !c.downloaded.WaitForTrue(30 * 1000) {
		return DownloadFailed
	}
	return nil
}

func (c *qfixConnector) CreateOrder(order *Order) (OrderID, error) {
	if !c.loggedIn.IsTrue() {
		return -1, NotConnected
	}

	if order.OrderType != Limit && order.OrderType != Market {
		return -1, UnsupportedOrderType
	}

	c.nextOrder = c.nextOrder + 1

	var orderID = OrderID(c.nextOrder)

	c.orders.Store(orderID, order)

	order.Id = orderID

	var ordtype = field.NewOrdType(enum.OrdType_LIMIT)
	if order.OrderType == Market {
		ordtype = field.NewOrdType(enum.OrdType_MARKET)
	}

	fixOrder := newordersingle.New(field.NewClOrdID(orderID.String()), field.NewSide(MapToFixSide(order.Side)), field.NewTransactTime(time.Now()), ordtype)
	fixOrder.SetSymbol(order.Instrument.Symbol())
	fixOrder.SetOrderQty(ToDecimal(order.Quantity), 4)
	fixOrder.SetPrice(ToDecimal(order.Price), 4)

	return orderID, quickfix.SendToTarget(fixOrder, c.sessionID)
}

func (c *qfixConnector) ModifyOrder(id OrderID, price Fixed, quantity Fixed) error {
	if !c.loggedIn.IsTrue() {
		return NotConnected
	}
	order := c.GetOrder(id)
	if order == nil {
		return OrderNotFound
	}
	order.Lock()
	defer order.Unlock()

	order.Price = price
	order.Quantity = quantity

	var ordtype = field.NewOrdType(enum.OrdType_LIMIT)

	// the GOX allows re-using of ClOrdID, similar to CME
	msg := ordercancelreplacerequest.New(field.NewOrigClOrdID(id.String()), field.NewClOrdID(id.String()), field.NewSide(MapToFixSide(order.Side)), field.NewTransactTime(time.Now()), ordtype)

	msg.SetSymbol(order.Instrument.Symbol())
	msg.SetOrderQty(ToDecimal(order.Quantity), 4)
	msg.SetPrice(ToDecimal(order.Price), 4)

	return quickfix.SendToTarget(msg, c.sessionID)
}

func (c *qfixConnector) CancelOrder(id OrderID) error {
	if !c.loggedIn.IsTrue() {
		return NotConnected
	}
	order := c.GetOrder(id)
	if order == nil {
		return OrderNotFound
	}
	order.Lock()
	defer order.Unlock()

	msg := ordercancelrequest.New(field.NewOrigClOrdID(id.String()), field.NewClOrdID(id.String()), field.NewSide(MapToFixSide(order.Side)), field.NewTransactTime(time.Now()))

	return quickfix.SendToTarget(msg, c.sessionID)
}

func (c *qfixConnector) Quote(instrument Instrument, bidPrice Fixed, bidQuantity Fixed, askPrice Fixed, askQuantity Fixed) error {

	if !c.loggedIn.IsTrue() {
		return NotConnected
	}

	c.nextQuote += 1
	m := massquote.New(field.NewQuoteID("1"))
	qsg := massquote.NewNoQuoteSetsRepeatingGroup()

	qs := qsg.Add()
	qs.SetQuoteSetID("1")

	qeg := massquote.NewNoQuoteEntriesRepeatingGroup()
	qe := qeg.Add()

	qe.SetQuoteEntryID(instrument.Symbol())
	qe.SetSymbol(instrument.Symbol())
	qe.SetBidSize(ToDecimal(bidQuantity), 4)
	qe.SetBidPx(ToDecimal(bidPrice), 4)
	qe.SetOfferSize(ToDecimal(askQuantity), 4)
	qe.SetOfferPx(ToDecimal(askPrice), 4)

	qs.SetNoQuoteEntries(qeg)
	m.SetNoQuoteSets(qsg)

	return quickfix.SendToTarget(m, c.sessionID)
}

func (c *qfixConnector) GetExchangeCode() string {
	return "GOT"
}
func (c *qfixConnector) GetOrder(id OrderID) *Order {
	_order, ok := c.orders.Load(id)
	if !ok {
		return nil
	}
	return _order.(*Order)
}

func NewConnector(callback ConnectorCallback, props Properties, logOutput io.Writer) ExchangeConnector {
	if logOutput == nil {
		logOutput = os.Stdout
	}

	filename := props.GetString("fix", "")
	c := &qfixConnector{settings: filename, log: logOutput}
	c.callback = callback

	return c
}
