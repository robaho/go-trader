package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector"
	"github.com/shopspring/decimal"
)

type algoState int

const (
	preInstrument algoState = iota
	preEntry
	waitBuy
	waitExit
	waitSell
	preExit
)

var exchange ExchangeConnector

type MyAlgo struct {
	symbol      string
	instrument  Instrument
	entryPrice  decimal.Decimal
	offset      decimal.Decimal
	totalProfit decimal.Decimal
	state       algoState
	runs        int
	nextEntry   time.Time
}

func (a *MyAlgo) OnBook(book *Book) {
	if book.Instrument != a.instrument {
		return
	}

	fmt.Println(book)

	switch a.state {
	case preEntry:
		if time.Now().Before(a.nextEntry) {
			return
		}
		if book.HasAsks() {
			exchange.CreateOrder(LimitOrder(a.instrument, Buy, book.Asks[0].Price, NewDecimal("1")))
			a.state = waitBuy
			a.runs++
		}
	case waitExit:
		if book.HasBids() {
			price := book.Bids[0].Price
			if price.GreaterThanOrEqual(a.entryPrice.Add(a.offset)) { // exit winner
				exchange.CreateOrder(MarketOrder(a.instrument, Sell, NewDecimal("1")))
				a.state = waitSell
			} else if price.LessThanOrEqual(a.entryPrice.Sub(a.offset)) { // exit loser ( 2 x the offset )
				exchange.CreateOrder(MarketOrder(a.instrument, Sell, NewDecimal("1")))
				a.state = waitSell
			}
		}
	}
}

func (a *MyAlgo) OnInstrument(instrument Instrument) {
	if a.state == preInstrument && instrument.Symbol() == a.symbol {
		a.instrument = instrument
		a.state = preEntry
		fmt.Println("assigned instrument")
	}
}

func (*MyAlgo) OnOrderStatus(order *Order) {
}

func (a *MyAlgo) OnFill(fill *Fill) {
	if a.state == waitBuy {
		a.entryPrice = fill.Price
		fmt.Println("entered market at ", fill.Price)
		a.state = waitExit
	}
	if a.state == waitSell {
		profit := fill.Price.Sub(a.entryPrice)
		fmt.Println("exited market at ", fill.Price)
		if profit.GreaterThan(decimal.Zero) {
			fmt.Println("!!!! winner ", profit)
		} else {
			fmt.Println("____ loser ", profit)
		}
		a.totalProfit = a.totalProfit.Add(profit)
		a.state = preEntry
		a.nextEntry = time.Now().Add(time.Second)
	}
	//fmt.Println("fill", fill, "total profit",a.totalProfit)
}

func (*MyAlgo) OnTrade(trade *Trade) {
	//fmt.Println("trade", trade)
}

//
// simple "algo" that buys a instrument, with an exit price offset - double the offset for exiting a loser.
// It tracks and reports total profit every 10 seconds.
// Very simple since it only handles an initial buy with quantities of 1.
//
func main() {
	var callback = MyAlgo{state: preInstrument}

	symbol := flag.String("symbol", "IBM", "set the symbol")
	fix := flag.String("fix", "configs/qf_algo_settings", "set the fix session file")
	props := flag.String("props", "configs/got_settings", "set exchange properties file")
	offset := flag.Float64("offset", 1.0, "price offset for entry exit")

	flag.Parse()

	callback.symbol = *symbol
	callback.offset = decimal.NewFromFloat(*offset)

	p, err := NewProperties(*props)
	if err != nil {
		panic(err)
	}
	p.SetString("fix", *fix)

	exchange = connector.NewConnector(&callback, p, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	err := exchange.DownloadInstruments()
	if err != nil {
		panic(err)
	}

	instrument := IMap.GetBySymbol(callback.symbol)
	if instrument == nil {
		log.Fatal("unable symbol", symbol)
	}

	fmt.Println("running algo on", instrument.Symbol(), "...")

	for {
		time.Sleep(time.Duration(10) * time.Second)
		tp := callback.totalProfit
		if tp.LessThan(decimal.Zero) {
			fmt.Println("<<<<< total profit", tp)
		} else {
			fmt.Println(">>>>> total profit", tp)
		}
	}
}
