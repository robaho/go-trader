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
	instrumentWG *sync.WaitGroup
}

func (cb *MyCallback) OnBook(book *Book) {
	if book.Instrument.Symbol() == cb.symbol {
		cb.ch <- true
	}
}

func (cb *MyCallback) OnInstrument(instrument Instrument) {
	if cb.instrumentWG!=nil {
		cb.instrumentWG.Done()
	}
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

	symbol := flag.String("symbol", "", "set the symbol")
	symbols := flag.String("symbols", "", "set the comma delimited list of symbols")
	bench := flag.Int("bench", 0, "benchmark market maker using N symbols (n > 0)")
	fix := flag.String("fix", "configs/qf_connector_settings", "set the fix session file")
	props := flag.String("props", "configs/got_settings", "set exchange properties file")
	delay := flag.Int("delay", 0, "set the delay in ms after each quote, 0 to disable")
	proto := flag.String("proto", "", "override protocol, grpc or fix")
	duration := flag.Int("duration", 0, "run for N seconds, 0 = forever")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	senderCompID := flag.String("id", "MM", "set the SenderCompID")
	symbolAsCompID := flag.Bool("sid", false, "use symbol as SenderCompID")
	mdbs := flag.String("mdbs", "", "market data buffer size (e.g. 1M, 16384, etc.)")

	flag.Parse()

	quotedSymbols := make([]string,0);

	if *symbols!="" {
		quotedSymbols = append(quotedSymbols,strings.Split(*symbols,",")...)
	} else if *symbol!="" {
		quotedSymbols = append(quotedSymbols,*symbol)
	} else if *bench!=0 {
		for i := range *bench {
			quotedSymbols = append(quotedSymbols,fmt.Sprint("S",i+1))
		}
	}

	fmt.Println("quoted symbols:",strings.Join(quotedSymbols,","))

	if len(quotedSymbols)==0 {
		panic("most provide either symbols, symbol, or bench")
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
	if *mdbs!="" {
		p.SetString("marketdata_buffer", *mdbs)
	}

	if *bench > 0 {
		createSymbols(p,quotedSymbols)
	}

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
	fmt.Println("all quoters completed")
}

func createSymbols(p Properties,symbols []string) {
	var wg = sync.WaitGroup{}
	var callback = MyCallback{ch: make(chan bool, 128), symbol: "?", instrumentWG: &wg}
	var exchange = connector.NewConnector(&callback, p, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}
	for _,s := range symbols {
		wg.Add(1)
		exchange.CreateInstrument(s)
	}
	wg.Wait()
	fmt.Println("created",len(symbols),"instruments")
	exchange.Disconnect()
}

func quoteSymbol(symbol string, p Properties,duration int, delay int,symbolAsCompID bool,wg *sync.WaitGroup) {
	defer wg.Done()
	p = p.Clone()

	if symbolAsCompID {
		fmt.Println("setting senderCompID to",symbol)
		p.SetString("senderCompID", symbol)
	}

	var callback = MyCallback{ch:make(chan bool, 128), symbol: symbol}

	var exchange = connector.NewConnector(&callback, p, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}
	defer exchange.Disconnect()

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
			for len(callback.ch) > 0 {
				<-callback.ch
			}
		}
		h.Add(float64(time.Since(now).Nanoseconds()))
		if delay != 0 {
			time.Sleep(time.Duration(int64(delay)) * time.Millisecond)
		}
		atomic.AddUint64(&updates, 1)
		seconds := time.Since(start).Seconds()
		if seconds > 10 {
			fmt.Printf("%s updates per second %d, avg rtt %dus, 10%% rtt %dus 99%% rtt %dus\n", symbol, updates/uint64(seconds), int(h.Mean()/1000.0), int(h.Quantile(.10)/1000.0), int(h.Quantile(.99)/1000.0))
			updates = 0
			start = time.Now()
			h = gohistogram.NewHistogram(50)
		}
	}
	fmt.Println("completed sending quotes on", instrument.Symbol())
}
