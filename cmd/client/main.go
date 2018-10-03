package main

import (
	"flag"
	"fmt"
	"github.com/shopspring/decimal"
	"log"
	"strings"
	"sync"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector"
	"github.com/robaho/gocui"
)

var gui *gocui.Gui
var activeOrderLock = sync.RWMutex{}
var activeOrders = make(map[OrderID]*Order)
var exchange ExchangeConnector
var trackingBook Instrument

type MyCallback struct {
}

func (MyCallback) OnBook(book *Book) {
	bidPrice, bidQty, askPrice, askQty := "", "", "", ""
	if book.HasBids() {
		bidPrice = book.Bids[0].Price.StringFixed(2)
		bidQty = book.Bids[0].Quantity.String()
	}
	if book.HasAsks() {
		askPrice = book.Asks[0].Price.StringFixed(2)
		askQty = book.Asks[0].Quantity.String()
	}

	vlogf("markets", "%10s %5s @ %12s / %5s @ %12s\n", book.Instrument.Symbol(), bidQty, bidPrice, askQty, askPrice)

	if book.Instrument == trackingBook {
		gui.Update(func(g *gocui.Gui) error {
			v, _ := g.View("book")
			v.Clear()
			v.FgColor = gocui.ColorRed
			for i := len(book.Asks) - 1; i >= 0; i-- {
				fmt.Fprintf(v, "%5s @ %10s\n", book.Asks[i].Quantity.String(), book.Asks[i].Price.StringFixed(2))
			}
			v.FgColor = gocui.ColorGreen
			for i := 0; i < len(book.Bids); i++ {
				fmt.Fprintf(v, "%5s @ %10s\n", book.Bids[i].Quantity.String(), book.Bids[i].Price.StringFixed(2))
			}
			v.FgColor = gocui.ColorDefault
			return nil
		})
	}
}

func vlogf(view string, format string, a ...interface{}) {
	vlogcf(view, gocui.ColorDefault, format, a...)
}

func vlogcf(view string, color gocui.Attribute, format string, a ...interface{}) {
	gui.Update(func(g *gocui.Gui) error {
		v, err := g.View(view)
		if err != nil {
			return err
		}
		v.FgColor = color
		_, err = fmt.Fprintf(v, format, a...)
		v.FgColor = gocui.ColorDefault
		return err
	})
}

func vlogln(view string, a ...interface{}) {
	gui.Update(func(g *gocui.Gui) error {
		v, err := g.View(view)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(v, a...)
		return err
	})
}

func (MyCallback) OnInstrument(instrument Instrument) {
	vlogf("log", "received instrument %s\n", instrument.Symbol())
}

func (MyCallback) OnOrderStatus(order *Order) {
	if order.IsActive() {
		activeOrderLock.Lock()
		defer activeOrderLock.Unlock()
		activeOrders[order.Id] = order
	} else {
		activeOrderLock.Lock()
		defer activeOrderLock.Unlock()
		delete(activeOrders, order.Id)
		vlogf("log", "order %d is %s\n", order.Id, order.OrderState)
	}
	gui.Update(func(g *gocui.Gui) error {
		v, err := g.View("orders")
		if err != nil {
			return err
		}
		v.Clear()
		activeOrderLock.Lock()
		defer activeOrderLock.Unlock()
		for _, order := range activeOrders {
			color := gocui.ColorGreen
			if order.Side == Sell {
				color = gocui.ColorRed
			}
			v.FgColor = color

			qty := order.Remaining.String()
			if !order.Remaining.Equals(order.Quantity) {
				qty = qty + " (" + order.Quantity.String() + ")"
			}
			fmt.Fprintf(v, "%5d %10s %5s %10s @ %10s\n", order.Id, order.Instrument.Symbol(), order.Side, qty, order.Price.StringFixed(2))
			v.FgColor = gocui.ColorDefault
		}
		return err
	})

}

func (MyCallback) OnFill(fill *Fill) {
	color := gocui.ColorGreen
	if fill.Side == Sell {
		color = gocui.ColorRed
	}
	if fill.IsQuote {
		vlogcf("fills", color, "quote fill on %s, %s %s @ %s\n", fill.Instrument.Symbol(), fill.Side, fill.Quantity.String(), fill.Price.StringFixed(2))
	} else {
		vlogcf("fills", color, "order %d fill on %s, %s %s @ %s\n", fill.Order.Id, fill.Instrument.Symbol(), fill.Side, fill.Quantity.String(), fill.Price.StringFixed(2))
	}
}

var lastPrice = map[Instrument]decimal.Decimal{}

func (MyCallback) OnTrade(trade *Trade) {
	color := gocui.ColorWhite

	lp, ok := lastPrice[trade.Instrument]
	if ok {
		if trade.Price.Equals(lp) {
			color = gocui.ColorWhite
		} else if trade.Price.GreaterThan(lp) {
			color = gocui.ColorGreen
		} else {
			color = gocui.ColorRed
		}
	}
	lastPrice[trade.Instrument] = trade.Price

	vlogcf("markets", color, "trade on %s, %s @ %s\n", trade.Instrument.Symbol(), trade.Quantity.String(), trade.Price.StringFixed(2))
}

type viewLogger struct{}

func (viewLogger) Write(p []byte) (n int, err error) {
	gui.Update(func(g *gocui.Gui) error {
		v, err := g.View("log")
		if err != nil {
			return err
		}
		_, err = v.Write(p)
		return err
	})
	return len(p), nil
}

var MyEditor gocui.Editor = gocui.EditorFunc(simpleEditor)

// simpleEditor is used as the default gocui editor.
func simpleEditor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyDelete:
		v.EditDelete(false)
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	var cols [4]int
	cols[0] = 0
	cols[1] = int(float32(maxX) * 0.20)
	cols[2] = int(float32(maxX) * 0.60)
	cols[3] = maxX - 1

	var rows [4]int
	rows[0] = 0
	rows[1] = (maxY - 8) / 2
	rows[2] = maxY - 8
	rows[3] = maxY - 1

	if v, err := g.SetView("log", cols[0], rows[0], cols[1], rows[2]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Log"
		v.MaxLines = 1000
		v.Wrap = true
	}
	if v, err := g.SetView("orders", cols[1], rows[0], cols[2], rows[1]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Active Orders"
	}
	if v, err := g.SetView("fills", cols[1], rows[1], cols[2], rows[2]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.MaxLines = 1000
		v.Autoscroll = true
		v.Title = "Order Fills"
	}
	if v, err := g.SetView("markets", cols[2], rows[0], cols[3], rows[1]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.MaxLines = 1000
		v.Autoscroll = true
		v.Title = "Streaming Markets"
	}
	if v, err := g.SetView("book", cols[2], rows[1], cols[3], rows[2]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Selected Book"
	}

	if v, err := g.SetView("commands", cols[0], rows[2], cols[3], rows[3]); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Commands"
		v.Editable = true
		v.Editor = MyEditor
		v.Wrap = true
		v.Autoscroll = true
		fmt.Fprintln(v, "Enter 'help' for list of commands")
		printCommand(v)
		g.Update(scrollToEnd)
		//v.Wrap = true
		if _, err := g.SetCurrentView("commands"); err != nil {
			return err
		}
	}

	return nil
}

func scrollToEnd(g *gocui.Gui) error {
	v, err := g.View("commands")
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		} else {
			return nil // view not ready yet
		}
	}

	nlines := len(v.ViewBufferLines())
	_, oy := v.Origin()

	if nlines > 0 {
		line := v.ViewBufferLines()[nlines-1]
		v.SetCursor(len(line), nlines-oy-1)
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
func processCommand(g *gocui.Gui, v *gocui.View) error {
	cmd := strings.TrimSpace(v.ViewBufferLines()[len(v.ViewBufferLines())-1])

	fmt.Fprintf(v, "\n")

	if strings.HasPrefix(cmd, "Command?") {
		cmd = cmd[8:]
		cmd = strings.TrimSpace(cmd)
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		goto again
	}
	if "help" == parts[0] {
		fmt.Fprintln(v, "The available commands are: quit, {buy:sell} SYMBOL QTY [PRICE], modify ORDERID QTY PRICE, cancel ORDERID, book SYMBOL")
	} else if "quit" == parts[0] {
		return gocui.ErrQuit
	} else if ("buy" == parts[0] || "sell" == parts[0]) && (len(parts) == 4 || len(parts) == 3) {
		instrument := IMap.GetBySymbol(parts[1])
		if instrument == nil {
			fmt.Fprintln(v, "unknown instrument", parts[1])
			goto again
		}
		qty := NewDecimal(parts[2])

		var side Side
		if "buy" == parts[0] {
			side = Buy
		} else if "sell" == parts[0] {
			side = Sell
		} else {
			fmt.Fprintln(v, "incorrect buy/sell type", parts[1])
			goto again
		}
		var err error
		if len(parts) == 4 {
			price := NewDecimal(parts[3])
			order := LimitOrder(instrument, side, price, qty)
			_, err = exchange.CreateOrder(order)
		} else {
			order := MarketOrder(instrument, side, qty)
			_, err = exchange.CreateOrder(order)
		}
		if err != nil {
			vlogf("log", "unable to submit order %s\n", err.Error())
		}

	} else if "modify" == parts[0] && len(parts) == 4 {
		orderID := NewOrderID(parts[1])
		qty := NewDecimal(parts[2])
		price := NewDecimal(parts[3])
		err := exchange.ModifyOrder(orderID, price, qty)
		if err != nil {
			vlogln("log", "unable to modify", err)
		}
	} else if "cancel" == parts[0] && len(parts) == 2 {
		orderID := NewOrderID(parts[1])
		err := exchange.CancelOrder(orderID)
		if err != nil {
			vlogln("log", "unable to cancel", err)
		}
	} else if "book" == parts[0] && len(parts) == 2 {
		instrument := IMap.GetBySymbol(parts[1])
		if instrument == nil {
			fmt.Fprintln(v, "unknown instrument ", parts[1])
		} else {
			trackingBook = instrument
			v, _ := g.View("book")
			v.Title = "Book Depth for " + instrument.Symbol()
		}
	} else {
		fmt.Fprintln(v, "Unknown command, '", cmd, "' use 'help'")
	}

again:
	printCommand(v)

	g.Update(scrollToEnd)

	return nil
}

func printCommand(v *gocui.View) {
	v.FgColor = gocui.AttrBold
	fmt.Fprint(v, "Command?")
	v.FgColor = gocui.ColorDefault
}

func main() {
	var callback = MyCallback{}

	fix := flag.String("fix", "configs/qf_client_settings", "set the fix session file")

	flag.Parse()

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	g.EscapeProcessing = false // we manage our own colors
	defer g.Close()

	gui = g

	g.SetManagerFunc(layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("commands", gocui.KeyEnter, gocui.ModNone, processCommand); err != nil {
		log.Panicln(err)
	}

	exchange = connector.NewConnector(callback, *fix, viewLogger{})

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}
