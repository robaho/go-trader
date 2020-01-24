package exchange

import (
	"fmt"
	. "github.com/robaho/fixed"
	"log"
	"strconv"

	"github.com/pkg/errors"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/protocol"
)

type grpcServer struct {
	e *exchange
}

type grpcClient struct {
	conn     protocol.Exchange_ConnectionServer
	loggedIn bool
	user     string
}

func (c *grpcClient) SendOrderStatus(so sessionOrder) {
	rpt := &protocol.ExecutionReport{}
	rpt.Symbol = so.order.Symbol()
	rpt.ExOrdId = so.order.ExchangeId
	rpt.ReportType = protocol.ExecutionReport_Status
	switch so.order.OrderState {
	case New, Booked:
		rpt.OrderState = protocol.ExecutionReport_Booked
	case PartialFill:
		rpt.OrderState = protocol.ExecutionReport_Partial
	case Filled:
		rpt.OrderState = protocol.ExecutionReport_Filled
	case Cancelled:
		rpt.OrderState = protocol.ExecutionReport_Cancelled
	case Rejected:
		rpt.OrderState = protocol.ExecutionReport_Rejected
	}
	rpt.RejectReason = so.order.RejectReason
	rpt.ClOrdId = int32(so.order.Id)
	rpt.Quantity = ToFloat(so.order.Quantity)
	rpt.Price = ToFloat(so.order.Price)
	rpt.Remaining = ToFloat(so.order.Remaining)
	if so.order.Side == Buy {
		rpt.Side = protocol.CreateOrderRequest_Buy
	} else {
		rpt.Side = protocol.CreateOrderRequest_Sell
	}
	reply := &protocol.OutMessage_Execrpt{Execrpt: rpt}
	so.client.(*grpcClient).conn.Send(&protocol.OutMessage{Reply: reply})
}

func (c *grpcClient) SendTrades(trades []trade) {
	for _, k := range trades {
		c.sendTradeExecutionReport(k.buyer, k.price, k.quantity, k.buyRemaining)
		c.sendTradeExecutionReport(k.seller, k.price, k.quantity, k.sellRemaining)
	}
}

func (c *grpcClient) SessionID() string {
	return fmt.Sprint(c.conn)
}

func (c *grpcClient) String() string {
	return c.SessionID()
}

func (c *grpcClient) sendTradeExecutionReport(so sessionOrder, price Fixed, quantity Fixed, remaining Fixed) {
	rpt := &protocol.ExecutionReport{}
	rpt.Symbol = so.order.Symbol()
	rpt.ExOrdId = so.order.ExchangeId
	rpt.ReportType = protocol.ExecutionReport_Fill
	rpt.ClOrdId = int32(so.order.Id)
	rpt.Quantity = ToFloat(so.order.Quantity)
	rpt.Price = ToFloat(so.order.Price)
	rpt.LastPrice = ToFloat(price)
	rpt.LastQuantity = ToFloat(quantity)
	if so.order.Side == Buy {
		rpt.Side = protocol.CreateOrderRequest_Buy
	} else {
		rpt.Side = protocol.CreateOrderRequest_Sell
	}
	switch so.order.OrderState {
	case New, Booked:
		rpt.OrderState = protocol.ExecutionReport_Booked
	case PartialFill:
		rpt.OrderState = protocol.ExecutionReport_Partial
	case Filled:
		rpt.OrderState = protocol.ExecutionReport_Filled
	case Cancelled:
		rpt.OrderState = protocol.ExecutionReport_Cancelled
	case Rejected:
		rpt.OrderState = protocol.ExecutionReport_Rejected
	}

	if !remaining.Equal(ZERO) {
		rpt.OrderState = protocol.ExecutionReport_Partial
	}

	rpt.Remaining = ToFloat(remaining)
	reply := &protocol.OutMessage_Execrpt{Execrpt: rpt}
	so.client.(*grpcClient).conn.Send(&protocol.OutMessage{Reply: reply})
}

func (s *grpcServer) Connection(conn protocol.Exchange_ConnectionServer) error {

	client := &grpcClient{conn: conn}

	s.e.newSession(client)

	log.Println("grpc session connect", client)
	defer func() {
		log.Println("grpc session disconnect", client)
		s.e.SessionDisconnect(client)
	}()

	for {
		msg, err := conn.Recv()

		if err != nil {
			log.Println("recv failed", err)
			return err
		}

		switch msg.Request.(type) {
		case *protocol.InMessage_Login:
			err = s.login(conn, client, msg.GetRequest().(*protocol.InMessage_Login).Login)
			if err != nil {
				return err
			}
			continue
		}
		if !client.loggedIn {
			reply := &protocol.OutMessage_Reject{Reject: &protocol.SessionReject{Error: "session not logged in"}}
			err = conn.Send(&protocol.OutMessage{Reply: reply})
			continue
		}
		switch msg.Request.(type) {
		case *protocol.InMessage_Download:
			s.download(conn, client)
		case *protocol.InMessage_Massquote:
			err = s.massquote(conn, client, msg.GetRequest().(*protocol.InMessage_Massquote).Massquote)
		case *protocol.InMessage_Create:
			err = s.create(conn, client, msg.GetRequest().(*protocol.InMessage_Create).Create)
		case *protocol.InMessage_Modify:
			err = s.modify(conn, client, msg.GetRequest().(*protocol.InMessage_Modify).Modify)
		case *protocol.InMessage_Cancel:
			err = s.cancel(conn, client, msg.GetRequest().(*protocol.InMessage_Cancel).Cancel)
		}

		if err != nil {
			log.Println("recv failed", err)
			return err
		}
	}
}
func (s *grpcServer) login(conn protocol.Exchange_ConnectionServer, client *grpcClient, request *protocol.LoginRequest) error {
	log.Println("login received", request)
	var err error = nil
	reply := &protocol.OutMessage_Login{Login: &protocol.LoginReply{Error: toErrS(err)}}
	err = conn.Send(&protocol.OutMessage{Reply: reply})
	client.loggedIn = true
	client.user = request.Username
	return err
}
func (s *grpcServer) download(conn protocol.Exchange_ConnectionServer, client *grpcClient) {
	log.Println("downloading...")
	for _, symbol := range IMap.AllSymbols() {
		instrument := IMap.GetBySymbol(symbol)
		sec := &protocol.OutMessage_Secdef{Secdef: &protocol.SecurityDefinition{Symbol: symbol, InstrumentID: instrument.ID()}}
		err := conn.Send(&protocol.OutMessage{Reply: sec})
		if err != nil {
			return
		}
	}
	sec := &protocol.OutMessage_Secdef{Secdef: &protocol.SecurityDefinition{Symbol: endOfDownload.Symbol(), InstrumentID: endOfDownload.ID()}}
	conn.Send(&protocol.OutMessage{Reply: sec})
	log.Println("downloading complete")
}
func (s *grpcServer) massquote(server protocol.Exchange_ConnectionServer, client *grpcClient, q *protocol.MassQuoteRequest) error {
	instrument := IMap.GetBySymbol(q.Symbol)
	if instrument == nil {
		return errors.New("unknown symbol " + q.Symbol)
	}
	return s.e.Quote(client, instrument, NewDecimalF(q.BidPrice), NewDecimalF(q.BidQuantity), NewDecimalF(q.AskPrice), NewDecimalF(q.AskQuantity))
}
func (s *grpcServer) create(conn protocol.Exchange_ConnectionServer, client *grpcClient, request *protocol.CreateOrderRequest) error {

	instrument := IMap.GetBySymbol(request.Symbol)
	if instrument == nil {
		reply := &protocol.OutMessage_Reject{Reject: &protocol.SessionReject{Error: "unknown symbol " + request.Symbol}}
		return conn.Send(&protocol.OutMessage{Reply: reply})
	}

	var order *Order
	var side Side

	if request.OrderSide == protocol.CreateOrderRequest_Buy {
		side = Buy
	} else {
		side = Sell
	}

	if request.OrderType == protocol.CreateOrderRequest_Limit {
		order = LimitOrder(instrument, side, NewDecimalF(request.Price), NewDecimalF(request.Quantity))
	} else {
		order = MarketOrder(instrument, side, NewDecimalF(request.Quantity))
	}
	order.Id = NewOrderID(strconv.Itoa(int(request.ClOrdId)))
	s.e.CreateOrder(client, order)
	return nil
}
func (s *grpcServer) modify(server protocol.Exchange_ConnectionServer, client *grpcClient, request *protocol.ModifyOrderRequest) error {
	price := NewDecimalF(request.Price)
	qty := NewDecimalF(request.Quantity)
	s.e.ModifyOrder(client, NewOrderID(strconv.Itoa(int(request.ClOrdId))), price, qty)
	return nil
}
func (s *grpcServer) cancel(server protocol.Exchange_ConnectionServer, client *grpcClient, request *protocol.CancelOrderRequest) error {
	s.e.CancelOrder(client, NewOrderID(strconv.Itoa(int(request.ClOrdId))))
	return nil
}

func toErrS(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func NewGrpcServer() protocol.ExchangeServer {
	s := grpcServer{e: &TheExchange}
	return &s
}
