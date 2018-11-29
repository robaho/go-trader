package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/connector"
)

type MyCallback struct {
}

func (*MyCallback) OnBook(book *Book) {
}

func (*MyCallback) OnInstrument(instrument Instrument) {
}

func (*MyCallback) OnOrderStatus(order *Order) {
}

func (*MyCallback) OnFill(fill *Fill) {
}

func (*MyCallback) OnTrade(trade *Trade) {
}

func main() {
	var callback = MyCallback{}

	fix := flag.String("fix", "configs/qf_playback_settings", "set the fix session file")
	props := flag.String("props", "configs/got_settings", "set exchange properties file")
	speed := flag.Float64("speed", 1.0, "set the playback speed")
	playback := flag.String("file", "playback.txt", "set the playback file")

	flag.Parse()

	p, err := NewProperties(*props)
	if err != nil {
		panic(err)
	}
	p.SetString("fix", *fix)

	var exchange = connector.NewConnector(&callback, p, nil)

	exchange.Connect()
	if !exchange.IsConnected() {
		panic("exchange is not connected")
	}

	err := exchange.DownloadInstruments()
	if err != nil {
		panic(err)
	}

	f, err := os.Open(*playback)
	if err != nil {
		panic(err)
	}

	r := bufio.NewReader(f)
	scanner := bufio.NewScanner(r)

	var lastTimestamp string

	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "#") {
			continue
		}
		parts := strings.Fields(s)
		if len(parts) != 6 {
			fmt.Println("invalid format", s)
			continue
		}
		timestamp := parts[0]
		symbol := parts[1]
		bidQty := NewDecimal(parts[2])
		bidPrice := NewDecimal(parts[3])
		askQty := NewDecimal(parts[4])
		askPrice := NewDecimal(parts[5])

		instrument := IMap.GetBySymbol(symbol)
		if instrument == nil {
			fmt.Println("unknown symbol", symbol)
			continue
		}

		duration, err := calcDuration(lastTimestamp, timestamp)
		if err != nil {
			fmt.Println("invalid timestamp", err)
			continue
		}
		exchange.Quote(instrument, bidPrice, bidQty, askPrice, askQty)

		if duration != 0 {
			time.Sleep(time.Duration(int64(float64(duration) / (*speed))))
		}
		lastTimestamp = timestamp
	}
}
func calcDuration(lastTimestamp string, timestamp string) (time.Duration, error) {
	if strings.HasPrefix(timestamp, "+") {
		return calcRelativeDuration(timestamp)
	}
	// have absolute timestamp, so previous must be absolute or empty
	if "" == lastTimestamp {
		return 0, nil
	}
	if !strings.HasPrefix(lastTimestamp, "+") {
		return 0, errors.New("previous timestamp must be absolute to use absolute timestamps")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return 0, err
	}
	last, err := strconv.ParseInt(lastTimestamp, 10, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(ts-last) * time.Millisecond, nil

}
func calcRelativeDuration(timestamp string) (time.Duration, error) {
	var suffix string
	var numeric string
	for i := 1; i < len(timestamp); i++ {
		if timestamp[i] >= '0' && timestamp[i] <= '9' {
			continue
		}
		suffix = timestamp[i:]
		numeric = timestamp[1:i]
		break
	}
	var d time.Duration
	switch suffix {
	case "us":
		d = time.Microsecond
	case "ms":
		d = time.Millisecond
	case "s":
		d = time.Second
	case "min":
		d = time.Minute
	}
	n, err := strconv.Atoi(numeric)
	if err != nil {
		return 0, err
	}
	return time.Duration(n) * d, nil
}
