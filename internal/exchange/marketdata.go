package exchange

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"fmt"
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
var eventChannel chan MarketEvent
var lastSentBook map[Instrument]uint64 // to avoid publishing exact same book multiple times due to coalescing
var sequence uint64
var udpCon *net.UDPConn

type MarketEvent struct {
	book   *Book
	trades []trade
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
	for {
		event := <-eventChannel

		book := getLatestBook(event.book)
		trades := coalesceTrades(event.trades)

		sendPacket(protocol.EncodeMarketEvent(book, trades))
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
	udpCon = c
	udpCon.SetWriteBuffer(16 * 1024 * 1024)

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
