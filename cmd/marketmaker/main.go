package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VividCortex/gohistogram"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector"
	"github.com/shopspring/decimal"
)

type MyCallback struct {
	cond   *sync.Cond
	symbol string
}

func (cb *MyCallback) OnBook(book *Book) {
	if book.Instrument.Symbol() == cb.symbol {
		cb.cond.Signal()
	}
}

func (*MyCallback) OnInstrument(instrument Instrument) {
}

func (*MyCallback) OnOrderStatus(order *Order) {
}

func (*MyCallback) OnFill(fill *Fill) {
	fmt.Println("fill", fill)
}

func (*MyCallback) OnTrade(trade *Trade) {
	fmt.Println("trade", trade)
}

func main() {
	var callback = MyCallback{}
	callback.cond = sync.NewCond(&sync.Mutex{})

	symbol := flag.String("symbol", "IBM", "set the symbol")
	fix := flag.String("fix", "configs/qf_mm1_settings", "set the fix session file")
	delay := flag.Int("delay", 0, "set the delay in ms after each quote, 0 to disable")

	flag.Parse()

	callback.symbol = *symbol

	var exchange = connector.NewConnector(&callback, *fix, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	instrument := IMap.GetBySymbol(callback.symbol)
	if instrument == nil {
		log.Fatal("unable symbol", symbol)
	}

	var updates uint64

	start := time.Now()

	fmt.Println("sending quotes on", instrument.Symbol(), "...")

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	bidPrice := NewDecimal("99")
	bidQty := NewDecimal("10")
	askPrice := NewDecimal("100")
	askQty := NewDecimal("10")

	lowLim := NewDecimal("75")
	highLim := NewDecimal("125")

	h := gohistogram.NewHistogram(50)

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

		now := time.Now()
		err := exchange.Quote(instrument, bidPrice, bidQty, askPrice, askQty)
		if err != nil {
			fmt.Println("unable to submit quote: " + err.Error())
		}
		callback.cond.L.Lock()
		callback.cond.Wait()
		callback.cond.L.Unlock()
		h.Add(float64(time.Now().Sub(now).Nanoseconds()))
		if *delay != 0 {
			time.Sleep(time.Duration(int64(*delay)) * time.Millisecond)
		}
		atomic.AddUint64(&updates, 1)
		if time.Now().Sub(start).Seconds() > 10 {
			fmt.Printf("updates per second %d, avg rtt %dus, 10%% rtt %dus 99%% rtt %dus\n", updates/10, int(h.Mean()/1000.0), int(h.Quantile(.10)/1000.0), int(h.Quantile(.99)/1000.0))
			updates = 0
			start = time.Now()
			h = gohistogram.NewHistogram(50)
		}
	}
}
