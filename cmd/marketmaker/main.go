package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VividCortex/gohistogram"
	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector"
)

type MyCallback struct {
	ch     chan bool
	symbol string
}

func (cb *MyCallback) OnBook(book *Book) {
	if book.Instrument.Symbol() == cb.symbol {
		cb.ch <- true
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

	symbol := flag.String("symbol", "IBM", "set the symbol")
	symbols := flag.String("symbols", "", "set the comma delimited list of symbols")
	fix := flag.String("fix", "configs/qf_connector_settings", "set the fix session file")
	props := flag.String("props", "configs/got_settings", "set exchange properties file")
	delay := flag.Int("delay", 0, "set the delay in ms after each quote, 0 to disable")
	proto := flag.String("proto", "", "override protocol, grpc or fix")
	duration := flag.Int("duration", 0, "run for N seconds, 0 = forever")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	senderCompID := flag.String("id", "MM", "set the SenderCompID")
	symbolAsCompID := flag.Bool("sid", false, "use symbol as SenderCompID")

	flag.Parse()

	quotedSymbols := make([]string,0);

	if(*symbols!="") {
		quotedSymbols = append(quotedSymbols,strings.Split(*symbols,",")...)
	} else if(*symbol!="") {
		quotedSymbols = append(quotedSymbols,*symbol)
	}

	fmt.Println("quoted symbols: ",strings.Join(quotedSymbols,","))

	if len(quotedSymbols)==0 {
		panic("most provide either symbols or symbol")
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	p, err := NewProperties(*props)
	if err != nil {
		panic(err)
	}
	if *proto != "" {
		p.SetString("protocol", *proto)
	}
	p.SetString("fix", *fix)
	p.SetString("senderCompID", *senderCompID)

	if len(quotedSymbols)>0 {
		// have to use symbol as senderCompID if quoting multiple symbols since multiple connections are used
		*symbolAsCompID=true;
	}

	var wg sync.WaitGroup

	for _,symbol := range quotedSymbols {
		wg.Add(1)
		go quoteSymbol(symbol,p,*duration,*delay,*symbolAsCompID, &wg)
	}
	wg.Wait()
}

func quoteSymbol(symbol string, p Properties,duration int, delay int,symbolAsCompID bool,wg *sync.WaitGroup) {
	defer wg.Done()
	p = p.Clone()

	if symbolAsCompID {
		fmt.Println("setting senderCompID to",symbol)
		p.SetString("senderCompID", symbol)
	}

	var callback = MyCallback{make(chan bool, 128), symbol}

	var exchange = connector.NewConnector(&callback, p, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	err := exchange.DownloadInstruments()
	if err != nil {
		panic(err)
	}

	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		log.Fatal("unknown symbol", symbol)
	}

	var updates uint64

	start := time.Now()
	end := start.Add(time.Duration(int64(duration)) * time.Second)

	fmt.Println("sending quotes on", instrument.Symbol(), "...")

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	bidPrice := NewDecimal("99.75")
	bidQty := NewDecimal("10")
	askPrice := NewDecimal("100")
	askQty := NewDecimal("10")

	lowLim := NewDecimal("75")
	highLim := NewDecimal("125")

	h := gohistogram.NewHistogram(50)

	for duration == 0 || time.Now().Before(end) {
		var delta = 1
		var r = r.Intn(10)
		if r <= 2 {
			delta = -1
		} else if r >= 7 {
			delta = 1
		} else {
			delta = 0
		}

		for {
			bidPrice = bidPrice.Add(NewDecimalF(float64(delta) * .25))
			askPrice = askPrice.Add(NewDecimalF(float64(delta) * .25))

			if bidPrice.LessThan(lowLim) {
				delta = 1
			} else if bidPrice.GreaterThan(highLim) {
				delta = -1
			} else {
				break
			}
		}

		now := time.Now()
		if delta != 0 {
			if bidPrice.Equal(askPrice) {
				panic("bid price equals ask price")
			}
			err := exchange.Quote(instrument, bidPrice, bidQty, askPrice, askQty)
			if err != nil {
				log.Fatal("unable to submit quote", err)
			}
			<-callback.ch
			// drain channel
			if len(callback.ch) > 0 {
				for range callback.ch {
				}
			}
		}
		h.Add(float64(time.Since(now).Nanoseconds()))
		if delay != 0 {
			time.Sleep(time.Duration(int64(delay)) * time.Millisecond)
		}
		atomic.AddUint64(&updates, 1)
		if time.Since(start).Seconds() > 10 {
			fmt.Printf("updates per second %d, avg rtt %dus, 10%% rtt %dus 99%% rtt %dus\n", updates/10, int(h.Mean()/1000.0), int(h.Quantile(.10)/1000.0), int(h.Quantile(.99)/1000.0))
			updates = 0
			start = time.Now()
			h = gohistogram.NewHistogram(50)
		}
	}
}
