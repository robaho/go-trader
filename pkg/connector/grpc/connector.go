package grpc

import (
	"context"
	"io"
	"log"
	"strings"
	"sync"

	. "github.com/robaho/fixed"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/protocol"
	"google.golang.org/grpc"
)

type grpcConnector struct {
	connected bool
	callback  ConnectorCallback
	nextOrder int64
	nextQuote int64
	// holds OrderID->*Order, concurrent since notifications/updates may arrive while order is being processed
	orders   sync.Map
	stream   protocol.Exchange_ConnectionClient
	addr     string
	loggedIn StatusBool
	// true after all instruments are downloaded from exchange
	downloaded StatusBool
	props      Properties
	log        io.Writer
}

type cachedConnection struct {
	refCount int
	conn *grpc.ClientConn
}

//TODO need reference count, since grpc.ClientConn is shared to allow for complete clean-up
var clients map[string]*cachedConnection = make(map[string]*cachedConnection)
var connectionLock = sync.Mutex{}

func (c *grpcConnector) IsConnected() bool {
	return c.connected
}

func (c *grpcConnector) Connect() error {
	connectionLock.Lock()
	defer connectionLock.Unlock()

	if c.connected {
		return AlreadyConnected
	}

	addr := c.props.GetString("grpc_host", "localhost") + ":" + c.props.GetString("grpc_port", "5000")
	c.addr = addr

	cached, ok := clients[addr]
	var conn *grpc.ClientConn
	var err error
	if !ok {
		conn, err = grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			return err
		}
		clients[addr]=&cachedConnection{conn: conn, refCount: 1}
	} else {
		conn = cached.conn
		cached.refCount++
	}

	client := protocol.NewExchangeClient(conn)

	//timeoutSecs := time.Second * time.Duration(timeout)

	ctx := context.Background()
	stream, err := client.Connection(ctx)

	if err != nil {
		conn.Close()
		return err
	}

	c.stream = stream

	log.Println("connection to exchange OK, sending login")

	request := &protocol.InMessage_Login{Login: &protocol.LoginRequest{Username: "guest", Password: "guest"}}

	err = stream.Send(&protocol.InMessage{Request: request})
	if err != nil {
		return err
	}

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				if !c.IsConnected() {
					return
				}
				log.Println("unable to receive message", err)
				c.Disconnect()
				return
			}

			switch msg.GetReply().(type) {
			case *protocol.OutMessage_Login:
				response := msg.GetReply().(*protocol.OutMessage_Login).Login
				if response.Error != "" {
					log.Println("unable to login", response.Error)
				} else {
					c.loggedIn.SetTrue()
				}
			case *protocol.OutMessage_Reject:
				response := msg.GetReply().(*protocol.OutMessage_Reject).Reject
				if response.Error != "" {
					log.Println("request rejected", response.Error)
				}
			case *protocol.OutMessage_Secdef:
				sec := msg.GetReply().(*protocol.OutMessage_Secdef).Secdef
				if sec.InstrumentID == 0 { // end of instrument download
					c.downloaded.SetTrue()
					continue
				}

				instrument := NewInstrument(int64(sec.InstrumentID), sec.Symbol)

				IMap.Put(instrument)

				c.callback.OnInstrument(instrument)
			case *protocol.OutMessage_Execrpt:
				rpt := msg.GetReply().(*protocol.OutMessage_Execrpt).Execrpt
				c.handleExecutionReport(rpt)
			}
		}
	}()

	// wait for login up to 30 seconds
	if !c.loggedIn.WaitForTrue(30 * 1000) {
		return ConnectionFailed
	}

	log.Println("login OK")

	c.connected = true

	return nil
}

func (c *grpcConnector) Disconnect() error {
	connectionLock.Lock()
	defer connectionLock.Unlock()

	if !c.connected {
		return NotConnected
	}

	c.stream.CloseSend()
	c.connected = false
	c.loggedIn.SetFalse()

	cached := clients[c.addr]
	cached.refCount--
	if cached.refCount==0 {
		delete(clients,c.addr)
	}

	return nil
}

func (c *grpcConnector) CreateInstrument(symbol string) {

	request := &protocol.InMessage_Secdefreq{Secdefreq: &protocol.SecurityDefinitionRequest{Symbol: symbol}}

	err := c.stream.Send(&protocol.InMessage{Request: request})
	if err != nil {
		log.Println("unable to send SecurityDefinitionRequest", err)
	}
}

func (c *grpcConnector) DownloadInstruments() error {
	if !c.loggedIn.IsTrue() {
		return NotConnected
	}

	c.downloaded.SetFalse()

	request := &protocol.InMessage_Download{Download: &protocol.DownloadRequest{}}

	err := c.stream.Send(&protocol.InMessage{Request: request})
	if err != nil {
		log.Println("unable to send DownloadRequest", err)
	}

	// wait for login up to 30 seconds
	if !c.downloaded.WaitForTrue(30 * 1000) {
		return DownloadFailed
	}
	return nil
}

func (c *grpcConnector) CreateOrder(order *Order) (OrderID, error) {
	if !c.loggedIn.IsTrue() {
		return -1, NotConnected
	}

	if order.OrderType != Limit && order.OrderType != Market {
		return -1, UnsupportedOrderType
	}

	c.nextOrder = c.nextOrder + 1

	var orderID = OrderID(c.nextOrder)
	order.Id = orderID
	c.orders.Store(orderID, order)

	co := protocol.CreateOrderRequest{}
	co.ClOrdId = int32(orderID)
	co.Symbol = order.Symbol()
	co.Price = ToFloat(order.Price)
	co.Quantity = ToFloat(order.Quantity)
	switch order.OrderType {
	case Market:
		co.OrderType = protocol.CreateOrderRequest_Market
	case Limit:
		co.OrderType = protocol.CreateOrderRequest_Limit
	}
	switch order.Side {
	case Buy:
		co.OrderSide = protocol.CreateOrderRequest_Buy
	case Sell:
		co.OrderSide = protocol.CreateOrderRequest_Sell
	}

	request := &protocol.InMessage_Create{Create: &co}
	err := c.stream.Send(&protocol.InMessage{Request: request})
	return orderID, err
}

func (c *grpcConnector) ModifyOrder(id OrderID, price Fixed, quantity Fixed) error {
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

	co := protocol.ModifyOrderRequest{}
	co.ClOrdId = int32(order.Id)
	co.Price = ToFloat(order.Price)
	co.Quantity = ToFloat(order.Quantity)

	request := &protocol.InMessage_Modify{Modify: &co}
	err := c.stream.Send(&protocol.InMessage{Request: request})
	return err
}

func (c *grpcConnector) CancelOrder(id OrderID) error {
	if !c.loggedIn.IsTrue() {
		return NotConnected
	}
	order := c.GetOrder(id)
	if order == nil {
		return OrderNotFound
	}
	order.Lock()
	defer order.Unlock()

	co := protocol.CancelOrderRequest{}
	co.ClOrdId = int32(order.Id)

	request := &protocol.InMessage_Cancel{Cancel: &co}
	err := c.stream.Send(&protocol.InMessage{Request: request})
	return err
}

func (c *grpcConnector) Quote(instrument Instrument, bidPrice Fixed, bidQuantity Fixed, askPrice Fixed, askQuantity Fixed) error {

	if !c.loggedIn.IsTrue() {
		return NotConnected
	}

	c.nextQuote += 1

	request := &protocol.InMessage_Massquote{Massquote: &protocol.MassQuoteRequest{
		Symbol:   instrument.Symbol(),
		BidPrice: ToFloat(bidPrice), BidQuantity: ToFloat(bidQuantity),
		AskPrice: ToFloat(askPrice), AskQuantity: ToFloat(askQuantity)}}

	err := c.stream.Send(&protocol.InMessage{Request: request})
	if err != nil {
		log.Println("unable to send MassQuote", err)
	}

	return err
}

func (c *grpcConnector) GetExchangeCode() string {
	return "GOT"
}
func (c *grpcConnector) GetOrder(id OrderID) *Order {
	_order, ok := c.orders.Load(id)
	if !ok {
		return nil
	}
	return _order.(*Order)
}

func (c *grpcConnector) handleExecutionReport(rpt *protocol.ExecutionReport) {
	exchangeId := rpt.ExOrdId
	var id OrderID
	var order *Order
	if strings.HasPrefix(exchangeId, "quote.") {
		// quote fill
		id = OrderID(0)
	} else {
		id = OrderID(int(rpt.ClOrdId))
		order = c.GetOrder(id)
		if order == nil {
			log.Println("unknown order ", id)
			return
		}
	}

	instrument := IMap.GetBySymbol(rpt.Symbol)
	if instrument == nil {
		log.Println("unknown symbol in execution report ", rpt.Symbol)
	}

	var state OrderState

	switch rpt.OrderState {
	case protocol.ExecutionReport_Booked:
		state = Booked
	case protocol.ExecutionReport_Cancelled:
		state = Cancelled
	case protocol.ExecutionReport_Partial:
		state = PartialFill
	case protocol.ExecutionReport_Filled:
		state = Filled
	case protocol.ExecutionReport_Rejected:
		state = Rejected
	}

	if order != nil {
		order.Lock()
		defer order.Unlock()

		order.ExchangeId = exchangeId
		order.Remaining = NewDecimalF(rpt.Remaining)
		order.Price = NewDecimalF(rpt.Price)
		order.Quantity = NewDecimalF(rpt.Quantity)

		order.OrderState = state
	}

	if rpt.ReportType == protocol.ExecutionReport_Fill {
		lastPx := NewF(rpt.LastPrice)
		lastQty := NewF(rpt.LastQuantity)

		var side Side
		if rpt.Side == protocol.CreateOrderRequest_Buy {
			side = Buy
		} else {
			side = Sell
		}

		fill := &Fill{Instrument: instrument, IsQuote: id == 0, Order: order, ExchangeID: exchangeId, Quantity: lastQty, Price: lastPx, Side: side, IsLegTrade: false}
		c.callback.OnFill(fill)
	}

	if order != nil {
		c.callback.OnOrderStatus(order)
	}

}

func NewConnector(callback ConnectorCallback, props Properties, logOutput io.Writer) ExchangeConnector {
	c := &grpcConnector{props: props, log: logOutput, callback: callback}
	return c
}
