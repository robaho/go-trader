package qfix

import (
	"fmt"
	"github.com/quickfixgo/fix44/securitydefinition"
	"strings"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/fix44/executionreport"
	"github.com/quickfixgo/quickfix"
	. "github.com/robaho/go-trader/pkg/common"
)

type myApplication struct {
	*quickfix.MessageRouter
	c *qfixConnector
}

func newApplication(c *qfixConnector) *myApplication {
	app := new(myApplication)
	app.MessageRouter = quickfix.NewMessageRouter()
	app.AddRoute(executionreport.Route(app.onExecutionReport))
	app.AddRoute(securitydefinition.Route(app.onSecurityDefinition))
	app.c = c
	return app
}

func (app *myApplication) OnCreate(sessionID quickfix.SessionID) {
}

func (app *myApplication) OnLogon(sessionID quickfix.SessionID) {
	if sessionID == app.c.sessionID {
		fmt.Fprintln(app.c.log, "we are logged in!")
		app.c.loggedIn.SetTrue()
	}
}

func (app *myApplication) OnLogout(sessionID quickfix.SessionID) {
	if sessionID == app.c.sessionID {
		fmt.Fprintln(app.c.log, "we are logged out!")
		app.c.loggedIn.SetFalse()
	}
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
	err := app.Route(message, sessionID)
	if err != nil {
		fmt.Fprintln(app.c.log, "error processing message", err)
	}
	return err
}

func (app *myApplication) onSecurityDefinition(msg securitydefinition.SecurityDefinition, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	_instrumentID, err := msg.GetSecurityID()
	if err != nil {
		return err
	}

	instrumentID := ParseInt(_instrumentID)

	symbol, err := msg.GetSymbol()
	if err != nil {
		return err
	}

	if instrumentID == 0 { // end of instrument download
		app.c.downloaded.SetTrue()
		return nil
	}

	instrument := NewInstrument(int64(instrumentID), symbol)

	IMap.Put(instrument)

	app.c.callback.OnInstrument(instrument)

	return nil
}

func (app *myApplication) onExecutionReport(msg executionreport.ExecutionReport, sessionID quickfix.SessionID) quickfix.MessageRejectError {

	exchangeId, err := msg.GetOrderID()
	if err != nil {
		return err
	}

	clOrdID, err := msg.GetClOrdID()
	if err != nil {
		return err
	}

	symbol, err := msg.GetSymbol()
	if err != nil {
		return err
	}

	var instrument Instrument
	var order *Order
	var id OrderID

	instrument = IMap.GetBySymbol(symbol)

	if strings.HasPrefix(exchangeId, "quote.") {
		// quote fill
		id = OrderID(0)
	} else {
		id = OrderID(ParseInt(clOrdID))
		order = app.c.GetOrder(id)
		if order == nil {
			return quickfix.NewMessageRejectError("unknown order clOrdID "+clOrdID, 0, nil)
		}
	}

	ordStatus, err := msg.GetOrdStatus()
	if err != nil {
		return err
	}

	remaining, err := msg.GetLeavesQty()
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

	execType, err := msg.GetExecType()
	if err != nil {
		return err
	}

	side, err := msg.GetSide()
	if err != nil {
		return err
	}

	if order != nil {
		order.Lock()
		defer order.Unlock()

		order.ExchangeId = exchangeId
		order.Remaining = ToFixed(remaining)
		order.Price = ToFixed(price)
		order.Quantity = ToFixed(qty)

		order.OrderState = MapFromFixOrdStatus(ordStatus)
	}

	if execType == enum.ExecType_FILL {
		lastPx, err := msg.GetLastPx()
		if err != nil {
			return err
		}
		lastQty, err := msg.GetLastQty()
		if err != nil {
			return err
		}
		fill := &Fill{instrument, id == 0, order, exchangeId, lastQty, lastPx, MapFromFixSide(side), false}
		app.c.callback.OnFill(fill)
	}

	if order != nil {
		app.c.callback.OnOrderStatus(order)
	}

	return nil
}
