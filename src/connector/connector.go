package connector

import (
	. "common"
	"exchange/protocol"
)

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/fix44/massquote"
	"github.com/quickfixgo/fix44/newordersingle"
	"github.com/quickfixgo/fix44/ordercancelreplacerequest"
	"github.com/quickfixgo/fix44/ordercancelrequest"
	"github.com/quickfixgo/quickfix"
	"github.com/shopspring/decimal"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type connector struct {
	connected bool
	callback  ConnectorCallback
	nextOrder int64
	nextQuote int64
	// holds OrderID->*Order, concurrent since notifications/updates may arrive while order is being processed
	orders    sync.Map
	sessionID quickfix.SessionID
	initiator *quickfix.Initiator
	loggedIn  StatusBool
	settings  string
	log       io.Writer
}

func (c *connector) IsConnected() bool {
	return c.connected
}

func (c *connector) Connect() error {
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

	//publish the known instruments, a real exchange would download these
	go func() {
		for _, s := range IMap.AllSymbols() {
			c.callback.OnInstrument(IMap.GetBySymbol(s))
		}
	}()

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

func (c *connector) Disconnect() error {
	if !c.connected {
		return NotConnected
	}
	c.initiator.Stop()
	c.connected = false
	return nil
}

func (c *connector) CreateOrder(order *Order) (OrderID, error) {
	if !c.loggedIn.IsTrue() {
		return -1, NotConnected
	}

	if order.OrderType != Limit {
		return -1, UnsupportedOrderType
	}

	c.nextOrder = c.nextOrder + 1

	var orderID = OrderID(c.nextOrder)

	c.orders.Store(orderID, order)

	order.Id = orderID

	var ordtype = field.NewOrdType(enum.OrdType_LIMIT)

	fixOrder := newordersingle.New(field.NewClOrdID(orderID.String()), field.NewSide(MapToFixSide(order.Side)), field.NewTransactTime(time.Now()), ordtype)
	fixOrder.SetSymbol(order.Instrument.Symbol())
	fixOrder.SetOrderQty(order.Quantity, 4)
	fixOrder.SetPrice(order.Price, 4)

	return orderID, quickfix.SendToTarget(fixOrder, c.sessionID)
}

func (c *connector) ModifyOrder(id OrderID, price decimal.Decimal, quantity decimal.Decimal) error {
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
	msg.SetOrderQty(order.Quantity, 4)
	msg.SetPrice(order.Price, 4)

	return quickfix.SendToTarget(msg, c.sessionID)
}

func (c *connector) CancelOrder(id OrderID) error {
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

func (c *connector) Quote(instrument Instrument, bidPrice decimal.Decimal, bidQuantity decimal.Decimal, askPrice decimal.Decimal, askQuantity decimal.Decimal) error {

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
	qe.SetBidSize(bidQuantity, 4)
	qe.SetBidPx(bidPrice, 4)
	qe.SetOfferSize(askQuantity, 4)
	qe.SetOfferPx(askPrice, 4)

	qs.SetNoQuoteEntries(qeg)
	m.SetNoQuoteSets(qsg)

	return quickfix.SendToTarget(m, c.sessionID)
}

func (c *connector) GetExchangeCode() string {
	return "GOT"
}
func (c *connector) GetOrder(id OrderID) *Order {
	_order, ok := c.orders.Load(id)
	if !ok {
		return nil
	}
	return _order.(*Order)
}

var replayRequests = make(chan protocol.ReplayRequest, 1000)

func NewConnector(callback ConnectorCallback, filename string, logOutput io.Writer) ExchangeConnector {
	if logOutput == nil {
		logOutput = os.Stdout
	}
	c := &connector{settings: filename, log: logOutput}
	c.callback = callback

	// read settings and create socket

	props := NewProperties("got_settings")
	saddr := props.GetString("multicast_addr", "")
	if saddr == "" {
		panic("unable to read multicast addr")
	}

	rhost := props.GetString("replay_host", "")
	if rhost == "" {
		panic("unable to read replay host")
	}

	rport := props.GetString("replay_port", "")
	if rport == "" {
		panic("unable to read replay port")
	}

	replayAddr := rhost + ":" + rport

	addr, err := net.ResolveUDPAddr("udp", saddr)
	if err != nil {
		panic(err)
	}

	go func() {
		var packetNumber uint64 = 0
		l, err := net.ListenMulticastUDP("udp", nil, addr)
		if err != nil {
			panic(err)
		}
		l.SetReadBuffer(16 * 1024 * 1024)
		for {
			b := make([]byte, 1024)
			_, _, err := l.ReadFromUDP(b)
			if err != nil {
				log.Fatal("ReadFromUDP failed:", err)
			}
			packetNumber = c.packetReceived(packetNumber, b)
		}
	}()

	go func() {
		var replaycon net.Conn = nil

		for {
			request := <-replayRequests
			if replaycon == nil {
				_replaycon, err := net.Dial("tcp", replayAddr)
				if err != nil {
					fmt.Fprintln(c.log, "unable to connect to replay host", err)
					continue
				}
				replaycon = _replaycon
				go func() {
					defer replaycon.Close()
					defer func() { replaycon = nil }()

					// just keep reading packets and applying them
					for {
						var len uint16
						err = binary.Read(replaycon, binary.LittleEndian, &len)
						if err != nil {
							fmt.Fprintln(c.log, "unable to read packet len", err)
							return
						}
						packet := make([]byte, len)
						n, err := replaycon.Read(packet)
						if err != nil || n != int(len) {
							fmt.Fprintln(c.log, "unable to read packet, expected", len, "received", n, err)
							return
						}
						c.processPacket(packet)
					}
				}()
			}

			err = binary.Write(replaycon, binary.LittleEndian, request)
		}
	}()

	return c
}
func (c *connector) packetReceived(expected uint64, buf []byte) uint64 {
	pn := binary.LittleEndian.Uint64(buf)
	if pn < expected {
		// server restart, reset the packet numbers
		expected = 0
	}

	if expected != 0 && pn != expected {
		// dropped some packets
		request := protocol.ReplayRequest{Start: expected, End: pn}
		replayRequests <- request
		fmt.Fprintln(c.log, "dropped packets from", expected, "to", pn)
	}

	c.processPacket(buf)
	return pn + 1
}

var lastSequence = make(map[Instrument]uint64)
var seqLock = sync.Mutex{}

func (c *connector) processPacket(packet []byte) {
	seqLock.Lock() // need locking because the main md go routine and the replay go routine call through here
	defer seqLock.Unlock()

	packet = packet[8:] // skip over packet number

	book, trades := protocol.DecodeMarketEvent(bytes.NewBuffer(packet))
	if book != nil {
		c.callback.OnBook(book)
	}
	for _, trade := range trades {
		c.callback.OnTrade(&trade)
	}
}
