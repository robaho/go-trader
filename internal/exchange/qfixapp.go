package exchange

import (
	"fmt"
	"sync"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/fix44/executionreport"
	"github.com/quickfixgo/fix44/massquote"
	"github.com/quickfixgo/fix44/newordersingle"
	"github.com/quickfixgo/fix44/ordercancelreplacerequest"
	"github.com/quickfixgo/fix44/ordercancelrequest"
	"github.com/quickfixgo/quickfix"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/shopspring/decimal"
)

var App myApplication

type myApplication struct {
	*quickfix.MessageRouter
	e          *exchange
	sessionIDs sync.Map
}

func (app *myApplication) OnCreate(sessionID quickfix.SessionID) {
}

func (app *myApplication) OnLogon(sessionID quickfix.SessionID) {
	s := newSession(sessionID.String())
	App.sessionIDs.Store(s.id, sessionID)
	app.e.sessions.Store(s.id, &s)
	fmt.Println("login, sessions are ", app.e.ListSessions())
}

func (app *myApplication) OnLogout(sessionID quickfix.SessionID) {
	app.e.SessionDisconnect(sessionID.String())
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
	app.e.CreateOrder(sessionID.String(), order)

	return nil

}
func (app *myApplication) onOrderCancelRequest(msg ordercancelrequest.OrderCancelRequest, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	clOrdId, err := msg.GetClOrdID()
	if err != nil {
		return err
	}
	app.e.CancelOrder(sessionID.String(), NewOrderID(clOrdId))

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
	app.e.ModifyOrder(sessionID.String(), NewOrderID(clOrdId), price, qty)

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

	app.e.Quote(sessionID.String(), instrument, bidPrice, bidQty, offerPrice, offerQty)

	return nil
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

	quickfix.SendToTarget(msg, getSessionID(so.session))
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

	quickfix.SendToTarget(msg, getSessionID(so.session))
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
}

func getSessionID(session string) quickfix.SessionID {
	id, _ := App.sessionIDs.Load(session)
	return id.(quickfix.SessionID)

}
