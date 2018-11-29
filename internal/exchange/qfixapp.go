package exchange

import (
	"fmt"
	"github.com/quickfixgo/fix44/securitydefinition"
	"github.com/quickfixgo/fix44/securitylistrequest"
	"strconv"
	"sync"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/fix44/executionreport"
	"github.com/quickfixgo/fix44/massquote"
	"github.com/quickfixgo/fix44/newordersingle"
	"github.com/quickfixgo/fix44/ordercancelreplacerequest"
	"github.com/quickfixgo/fix44/ordercancelrequest"
	"github.com/quickfixgo/fix44/securitydefinitionrequest"
	"github.com/quickfixgo/quickfix"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/shopspring/decimal"
)

var App myApplication
var endOfDownload = NewInstrument(0, "endofdownload")

type myApplication struct {
	*quickfix.MessageRouter
	e            *exchange
	lock         sync.Mutex
	instrumentID int64
}

type fixClient struct {
	sessionID quickfix.SessionID
}

func (c fixClient) SendOrderStatus(so sessionOrder) {
	App.sendExecutionReport(enum.ExecType_ORDER_STATUS, so)
}
func (c fixClient) SendTrades(trades []trade) {
	App.sendExecutionReports(trades)
}
func (c fixClient) SessionID() string {
	return c.sessionID.String()
}

func (app *myApplication) OnCreate(sessionID quickfix.SessionID) {
}

func (app *myApplication) OnLogon(sessionID quickfix.SessionID) {
	c := fixClient{sessionID: sessionID}
	app.e.newSession(c)
	fmt.Println("login, sessions are ", app.e.ListSessions())
}

func (app *myApplication) OnLogout(sessionID quickfix.SessionID) {
	c := fixClient{sessionID: sessionID}
	app.e.SessionDisconnect(c)
	fmt.Println("logout, sessions are ", app.e.ListSessions())
}

func (app *myApplication) ToAdmin(message *quickfix.Message, sessionID quickfix.SessionID) {
}

func (app *myApplication) ToApp(message *quickfix.Message, sessionID quickfix.SessionID) error {
	return nil
}

func (app *myApplication) FromAdmin(message *quickfix.Message, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	return nil
}

func (app *myApplication) FromApp(message *quickfix.Message, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	app.Route(message, sessionID)
	return nil
}

func (app *myApplication) onNewOrderSingle(msg newordersingle.NewOrderSingle, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	clOrdId, err := msg.GetClOrdID()
	if err != nil {
		return err
	}
	symbol, err := msg.GetSymbol()
	if err != nil {
		return err
	}
	side, err := msg.GetSide()
	if err != nil {
		return err
	}
	qty, err := msg.GetOrderQty()
	if err != nil {
		return err
	}
	ordType, err := msg.GetOrdType()
	if err != nil {
		return err
	}
	var price decimal.Decimal
	if ordType == enum.OrdType_LIMIT {
		price, err = msg.GetPrice()
		if err != nil {
			return err
		}
	}
	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		return quickfix.NewMessageRejectError("unknown symbol "+symbol, 0, nil)
	}
	var order *Order
	if ordType == enum.OrdType_LIMIT {
		order = LimitOrder(instrument, MapFromFixSide(side), price, qty)
	} else {
		order = MarketOrder(instrument, MapFromFixSide(side), qty)
	}
	order.Id = NewOrderID(clOrdId)

	c := fixClient{sessionID: sessionID}
	app.e.CreateOrder(c, order)

	return nil

}
func (app *myApplication) onOrderCancelRequest(msg ordercancelrequest.OrderCancelRequest, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	clOrdId, err := msg.GetClOrdID()
	if err != nil {
		return err
	}
	c := fixClient{sessionID: sessionID}
	app.e.CancelOrder(c, NewOrderID(clOrdId))

	return nil
}

func (app *myApplication) onOrderCancelReplaceRequest(msg ordercancelreplacerequest.OrderCancelReplaceRequest, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	clOrdId, err := msg.GetClOrdID()
	if err != nil {
		return err
	}
	price, err := msg.GetPrice()
	if err != nil {
		return err
	}
	qty, err := msg.GetOrderQty()
	if err != nil {
		return err
	}
	c := fixClient{sessionID: sessionID}
	app.e.ModifyOrder(c, NewOrderID(clOrdId), price, qty)

	return nil
}
func (app *myApplication) onMassQuote(msg massquote.MassQuote, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	rgNoQuoteSets, err := msg.GetNoQuoteSets()
	if err != nil {
		return err
	}
	if rgNoQuoteSets.Len() != 1 {
		return quickfix.NewBusinessMessageRejectError("only 1 quote set supported", 0, nil)
	}
	noQuoteSets := rgNoQuoteSets.Get(0)
	rgNoQuoteEntries, err := noQuoteSets.GetNoQuoteEntries()
	if rgNoQuoteEntries.Len() != 1 {
		return quickfix.NewBusinessMessageRejectError("only 1 quote supported", 0, nil)
	}
	noQuoteEntries := rgNoQuoteEntries.Get(0)

	symbol, err := noQuoteEntries.GetSymbol()
	if err != nil {
		return err
	}

	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		return quickfix.NewBusinessMessageRejectError("unknown symbol "+symbol, 0, nil)
	}
	bidPrice, err := noQuoteEntries.GetBidPx()
	if err != nil {
		return err
	}
	bidQty, err := noQuoteEntries.GetBidSize()
	if err != nil {
		return err
	}
	offerPrice, err := noQuoteEntries.GetOfferPx()
	if err != nil {
		return err
	}
	offerQty, err := noQuoteEntries.GetOfferSize()
	if err != nil {
		return err
	}

	c := fixClient{sessionID: sessionID}
	app.e.Quote(c, instrument, bidPrice, bidQty, offerPrice, offerQty)

	return nil
}

func (app *myApplication) onSecurityDefinitionRequest(msg securitydefinitionrequest.SecurityDefinitionRequest, sessionID quickfix.SessionID) quickfix.MessageRejectError {

	reqid, err := msg.GetSecurityReqID()
	if err != nil {
		return err
	}
	symbol, err := msg.GetSymbol()
	if err != nil {
		return err
	}

	app.lock.Lock()
	defer app.lock.Unlock()

	instrument := IMap.GetBySymbol(symbol)
	if instrument != nil {
		app.sendInstrument(instrument, reqid, sessionID)
	} else {
		app.instrumentID++
		instrument = NewInstrument(app.instrumentID, symbol)
		IMap.Put(instrument)
		app.sendInstrument(instrument, reqid, sessionID)
	}
	return nil
}

func (app *myApplication) onSecurityListRequest(msg securitylistrequest.SecurityListRequest, sessionID quickfix.SessionID) quickfix.MessageRejectError {

	reqid, err := msg.GetSecurityReqID()
	if err != nil {
		return err
	}

	for _, symbol := range IMap.AllSymbols() {
		instrument := IMap.GetBySymbol(symbol)
		app.sendInstrument(instrument, reqid, sessionID)
	}
	app.sendInstrument(endOfDownload, reqid, sessionID)

	return nil
}

func (app *myApplication) sendInstrument(instrument Instrument, reqid string, sessionID quickfix.SessionID) {
	resid := strconv.Itoa(int(instrument.ID()))
	restype := enum.SecurityResponseType_ACCEPT_SECURITY_PROPOSAL_WITH_REVISIONS_AS_INDICATED_IN_THE_MESSAGE
	msg := securitydefinition.New(field.NewSecurityReqID(reqid), field.NewSecurityResponseID(resid), field.NewSecurityResponseType(restype))

	msg.SetSymbol(instrument.Symbol())
	msg.SetSecurityID(strconv.FormatInt(instrument.ID(), 10))

	quickfix.SendToTarget(msg, sessionID)
}

func (app *myApplication) sendTradeExecutionReport(so sessionOrder, price decimal.Decimal, qty decimal.Decimal, remaining decimal.Decimal) {

	order := so.order

	var ordStatus enum.OrdStatus

	if remaining.Equals(ZERO) {
		ordStatus = MapToFixOrdStatus(order.OrderState)
	} else {
		ordStatus = enum.OrdStatus_PARTIALLY_FILLED
	}

	var side = MapToFixSide(order.Side)

	msg := executionreport.New(field.NewOrderID(order.ExchangeId),
		field.NewExecID(order.ExchangeId),
		field.NewExecType(enum.ExecType_FILL),
		field.NewOrdStatus(ordStatus),
		field.NewSide(side),
		field.NewLeavesQty(remaining, 4),
		field.NewCumQty(order.Quantity.Sub(remaining), 4),
		field.NewAvgPx(ZERO, 4))
	msg.SetClOrdID(order.Id.String())
	msg.SetPrice(order.Price, 4)
	msg.SetOrderQty(order.Quantity, 4)
	msg.SetSymbol(order.Instrument.Symbol())
	msg.SetLastPx(price, 4)
	msg.SetLastQty(qty, 4)

	quickfix.SendToTarget(msg, so.client.(fixClient).sessionID)
}

func (app *myApplication) sendExecutionReport(execType enum.ExecType, so sessionOrder) {

	order := so.order

	var side = MapToFixSide(order.Side)

	msg := executionreport.New(field.NewOrderID(order.ExchangeId),
		field.NewExecID(order.ExchangeId),
		field.NewExecType(execType),
		field.NewOrdStatus(MapToFixOrdStatus(order.OrderState)),
		field.NewSide(side),
		field.NewLeavesQty(order.Remaining, 4),
		field.NewCumQty(order.Quantity.Sub(order.Remaining), 4),
		field.NewAvgPx(ZERO, 4))
	msg.SetClOrdID(order.Id.String())
	msg.SetPrice(order.Price, 4)
	msg.SetOrderQty(order.Quantity, 4)
	msg.SetSymbol(order.Instrument.Symbol())

	quickfix.SendToTarget(msg, so.client.(fixClient).sessionID)
}

func (app *myApplication) sendExecutionReports(trades []trade) {
	for _, k := range trades {
		app.sendTradeExecutionReport(k.buyer, k.price, k.quantity, k.buyRemaining)
		app.sendTradeExecutionReport(k.seller, k.price, k.quantity, k.sellRemaining)
	}
}

func init() {
	App.e = &TheExchange

	App.MessageRouter = quickfix.NewMessageRouter()
	App.AddRoute(newordersingle.Route(App.onNewOrderSingle))
	App.AddRoute(ordercancelrequest.Route(App.onOrderCancelRequest))
	App.AddRoute(ordercancelreplacerequest.Route(App.onOrderCancelReplaceRequest))
	App.AddRoute(massquote.Route(App.onMassQuote))
	App.AddRoute(securitydefinitionrequest.Route(App.onSecurityDefinitionRequest))
	App.AddRoute(securitylistrequest.Route(App.onSecurityListRequest))

	App.instrumentID = 1000000 // start high for dynamic instruments
}
