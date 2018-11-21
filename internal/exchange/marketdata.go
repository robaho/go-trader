package exchange

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"fmt"
	"github.com/shopspring/decimal"
	"golang.org/x/net/ipv4"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/protocol"
)

// market data caches the latest books, and publishes books and exchange trades via multicast

var bookCache sync.Map
var statsCache sync.Map

var eventChannel chan MarketEvent
var lastSentBook map[Instrument]uint64 // to avoid publishing exact same book multiple times due to coalescing
var sequence uint64
var udpCon *net.UDPConn
var pUdpCon *ipv4.PacketConn
var subMutex sync.Mutex
var subscriptions []chan *Book

type MarketEvent struct {
	book   *Book
	trades []trade
}

type Statistics struct {
	Symbol   string
	BidQty   decimal.Decimal
	BidPrice decimal.Decimal
	AskQty   decimal.Decimal
	AskPrice decimal.Decimal
	Volume   decimal.Decimal
	High     decimal.Decimal
	HasHigh  bool
	Low      decimal.Decimal
	HasLow   bool
}

func subscribe(sub chan *Book) {
	subMutex.Lock()
	defer subMutex.Unlock()

	subscriptions = append(subscriptions, sub)
}

func unsubscribe(sub chan *Book) {
	subMutex.Lock()
	defer subMutex.Unlock()

	copy := subscriptions[:0]
	for _, v := range subscriptions {
		if v != sub {
			copy = append(copy, v)
		}
	}
	subscriptions = copy
}

func sendMarketData(event MarketEvent) {
	cacheBook(event.book)
	eventChannel <- event
}

func cacheBook(book *Book) {
	book.Sequence = atomic.AddUint64(&sequence, 1)
	bookCache.Store(book.Instrument, book)
}

func GetLatestBook(instrument Instrument) *Book {
	v, ok := bookCache.Load(instrument)
	if !ok {
		return nil
	}
	return v.(*Book)
}

func GetBook(symbol string) *Book {
	i := IMap.GetBySymbol(symbol)
	if i == nil {
		return nil
	}
	return GetLatestBook(i)
}

func publish() {
	stats := make(map[Instrument]*Statistics)

	for {
		event := <-eventChannel

		book := getLatestBook(event.book)
		trades := coalesceTrades(event.trades)

		s, ok := stats[book.Instrument]
		if !ok {
			s = &Statistics{}
			s.Symbol = book.Instrument.Symbol()
			stats[book.Instrument] = s
		}

		if book.HasBids() {
			s.BidPrice = book.Bids[0].Price
			s.BidQty = book.Bids[0].Quantity
		}
		if book.HasAsks() {
			s.AskPrice = book.Asks[0].Price
			s.AskQty = book.Asks[0].Quantity
		}

		for _, t := range trades {
			s.Volume = s.Volume.Add(t.Quantity)
			if !s.HasHigh || t.Price.GreaterThan(s.High) {
				s.High = t.Price
				s.HasHigh = true
			}
			if !s.HasLow || t.Price.LessThan(s.Low) {
				s.Low = t.Price
				s.HasLow = true
			}
		}

		statsCache.Store(book.Instrument, s)

		sendPacket(protocol.EncodeMarketEvent(book, trades))
		// publish to internal subscribers
		for _, sub := range subscriptions {
			sub <- book
		}
	}
}

func getLatestBook(book *Book) *Book {
	lastSeq, ok := lastSentBook[book.Instrument]
	if ok {
		if lastSeq >= book.Sequence {
			return nil
		}
	}
	return book
}

func getStatistics(instrument Instrument) *Statistics {
	stats, ok := statsCache.Load(instrument)
	if ok {
		return stats.(*Statistics)
	}
	return nil
}

func coalesceTrades(trades []trade) []Trade {
	var Trades []Trade

	// coalesce all trades at same price
	last := 0
	for i, v := range trades {
		if i > 0 && v.price.Equals(Trades[last].Price) {
			Trades[last].Quantity = Trades[last].Quantity.Add(v.quantity)
			continue
		}
		exchangeID := strconv.FormatInt(v.tradeid, 10)
		t := Trade{Instrument: v.seller.order.Instrument, Price: v.price, Quantity: v.quantity, ExchangeID: exchangeID, TradeTime: v.when}
		Trades = append(Trades, t)
		last = len(Trades) - 1
	}
	return Trades
}

var packetNumber uint64

func sendPacket(data []byte) {

	packetNumber++

	b := bytes.Buffer{}

	binary.Write(&b, binary.LittleEndian, packetNumber)
	b.Write(data)

	data = b.Bytes()

	_, err := udpCon.Write(data)
	if err != nil {
		fmt.Println("error sending packet", err)
	}

	rememberPacket(packetNumber, data)
}

func startMarketData() {
	eventChannel = make(chan MarketEvent)
	lastSentBook = make(map[Instrument]uint64)

	// read settings and create socket

	props, err := NewProperties("configs/got_settings")
	if err != nil {
		panic("unable to read multicast addr")
	}
	saddr := props.GetString("multicast_addr", "")
	if saddr == "" {
		panic("unable to read multicast addr")
	}
	intf := props.GetString("multicast_intf", "lo0")
	if intf == "" {
		panic("unable to read multicast addr")
	}
	_intf, err := net.InterfaceByName(intf)
	if err != nil {
		panic("unable to read multicast interface")
	}

	fmt.Println("publishing marketdata at", saddr)

	addr, err := net.ResolveUDPAddr("udp", saddr)
	if err != nil {
		panic(err)
	}

	rport := props.GetString("replay_port", "")
	if rport == "" {
		panic("unable to read replay port")
	}

	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}
	c.SetWriteBuffer(16 * 1024 * 1024)

	udpCon = c
	pUdpCon = ipv4.NewPacketConn(udpCon)
	pUdpCon.SetMulticastInterface(_intf)

	go func() {
		publish()
	}()

	go func() {
		ln, err := net.Listen("tcp", ":"+rport)
		if err != nil {
			log.Fatal("unable to listen on replay port", err)
		}
		for {
			conn, _ := ln.Accept()

			go func(conn net.Conn) {
				var request protocol.ReplayRequest
				for {
					err := binary.Read(conn, binary.LittleEndian, &request)
					if err != nil {
						return
					}
					err = resendPackets(conn, request)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()
}

type Packet struct {
	number uint64
	data   []byte
}
type PacketHistory struct {
	sync.RWMutex
	packets list.List
}

var history PacketHistory

func rememberPacket(packetNumber uint64, data []byte) {
	history.Lock()
	defer history.Unlock()

	p := new(Packet)
	p.number = packetNumber
	p.data = make([]byte, len(data))

	if history.packets.Len() > 10000 {
		history.packets.Remove(history.packets.Front())
	}

	packet := Packet{packetNumber, data}

	history.packets.PushBack(&packet)
}

func resendPackets(conn net.Conn, request protocol.ReplayRequest) error {
	history.RLock()
	defer history.RUnlock()

	expected := int(request.End - request.Start)
	var count = 0

	for e := history.packets.Front(); e != nil; e = e.Next() {
		p := e.Value.(*Packet)
		if p.number < request.Start {
			continue
		}
		if p.number >= request.End {
			break
		}
		count++
		var len = uint16(len(p.data))
		err := binary.Write(conn, binary.LittleEndian, &len)
		if err != nil {
			fmt.Println("unable to write replay packet header", err)
			return err
		}
		_, err = conn.Write(p.data)
		if err != nil {
			fmt.Println("unable to write replay packets", err)
			return err
		}
	}
	if count != expected {
		fmt.Println("replay failed", request, "missing", expected-count)
	} else {
		fmt.Println("replay complete", request, "count", count)
	}
	return nil
}
