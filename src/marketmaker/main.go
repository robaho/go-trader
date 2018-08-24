package main

import (
	. "common"
	"connector"
	"flag"
	"fmt"
	"github.com/shopspring/decimal"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type MyCallback struct {
	sync.WaitGroup
	symbol string
}

func (cb *MyCallback) OnBook(book *Book) {
	if book.Instrument.Symbol() == cb.symbol {
		cb.Done()
	}
}

func (*MyCallback) OnInstrument(instrument Instrument) {
	fmt.Println(instrument)
}

func (*MyCallback) OnOrderStatus(order *Order) {
	fmt.Println(order)
}

func (*MyCallback) OnFill(fill *Fill) {
	fmt.Println("fill", fill)
}

func (*MyCallback) OnTrade(trade *Trade) {
	fmt.Println("trade", trade)
}

func main() {
	var callback = MyCallback{}

	fileName := flag.String("fix", "qf_connector_settings", "set the fix session file")
	delay := flag.Int("delay", 0, "set the delay in ms after each quote, 0 to disable")

	flag.Parse()

	symbol := flag.Arg(0)
	if symbol == "" {
		log.Fatal("usage: marketmaker SYMBOL [-d/delay MS] [-fix qfsettingsfile]")
	}

	callback.symbol = symbol

	var exchange = connector.NewConnector(&callback, *fileName)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		log.Fatal("unable symbol", symbol)
	}

	var updates uint64

	fmt.Println("waiting to stabilize...")

	time.Sleep(3 * time.Second)

	start := time.Now()

	fmt.Println("sending quotes...")

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	bidPrice := NewDecimal("99")
	bidQty := NewDecimal("10")
	askPrice := NewDecimal("100")
	askQty := NewDecimal("10")

	lowLim := NewDecimal("75")
	highLim := NewDecimal("125")

	for {
		var delta = 1
		if r.Intn(10) < 5 {
			delta = -1
		}

		for {
			bidPrice = bidPrice.Add(decimal.NewFromFloat(float64(delta) * .25))
			askPrice = askPrice.Add(decimal.NewFromFloat(float64(delta) * .25))

			if bidPrice.LessThan(lowLim) {
				delta = 1
			} else if bidPrice.GreaterThan(highLim) {
				delta = -1
			} else {
				break
			}
		}

		callback.Add(1)
		err := exchange.Quote(instrument, bidPrice, bidQty, askPrice, askQty)
		if err != nil {
			fmt.Println("unable to submit quote: " + err.Error())
		}
		callback.Wait()
		if *delay != 0 {
			time.Sleep(time.Duration(int64(*delay)) * time.Millisecond)
		}
		atomic.AddUint64(&updates, 1)
		if time.Now().Sub(start).Seconds() > 10 {
			fmt.Println("updates per second", updates/10)
			updates = 0
			start = time.Now()
		}
	}
}
