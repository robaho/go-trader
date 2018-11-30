package marketdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	. "github.com/robaho/go-trader/pkg/common"
	"github.com/robaho/go-trader/pkg/protocol"
)

var replayRequests = make(chan protocol.ReplayRequest, 1000)

type marketDataReceiver struct {
	c        ExchangeConnector
	callback ConnectorCallback
	log      io.Writer
}

// StartMarketDataReceiver starts the multicast marketdata processor
func StartMarketDataReceiver(c ExchangeConnector, callback ConnectorCallback, props Properties, logOutput io.Writer) {
	// read settings and create socket

	md := marketDataReceiver{c: c, callback: callback, log: logOutput}

	saddr := props.GetString("multicast_addr", "")
	if saddr == "" {
		panic("unable to read multicast addr")
	}

	intf := props.GetString("multicast_intf", "lo0")
	if intf == "" {
		panic("unable to read multicast interface")
	}
	_intf, err := net.InterfaceByName(intf)
	if err != nil {
		panic(err)
	}

	rhost := props.GetString("replay_host", "")
	if rhost == "" {
		panic("unable to read replay host")
	}

	rport := props.GetString("replay_port", "")
	if rport == "" {
		panic("unable to read replay port")
	}

	replayAddr := rhost + ":" + rport

	addr, err := net.ResolveUDPAddr("udp", saddr)
	if err != nil {
		panic(err)
	}

	go func() {
		var packetNumber uint64 = 0
		l, err := net.ListenMulticastUDP("udp", _intf, addr)
		if err != nil {
			panic(err)
		}
		log.Println("listening for market data on", l.LocalAddr())
		l.SetReadBuffer(16 * 1024 * 1024)
		for {
			b := make([]byte, protocol.MaxMsgSize)
			n, _, err := l.ReadFromUDP(b)
			if err != nil {
				log.Fatal("ReadFromUDP failed:", err)
			}
			packetNumber = md.packetReceived(packetNumber, b[:n])
		}
	}()

	go func() {
		var replaycon net.Conn = nil
		var err error

		defer func() {
			log.Println("replay processor terminated")
		}()

		for {
			request := <-replayRequests
			if replaycon == nil {
				replaycon, err = net.Dial("tcp", replayAddr)
				if err != nil {
					fmt.Fprintln(md.log, "unable to connect to replay host", err)
					continue
				} else {
					fmt.Fprintln(md.log, "opened connection to replay host")
				}
				go func() {
					defer func() {
						replaycon.Close()
						replaycon = nil
					}()

					// just keep reading packets and applying them
					for {
						var len uint16
						err = binary.Read(replaycon, binary.LittleEndian, &len)
						if err != nil {
							fmt.Fprintln(md.log, "unable to read replay packet len", err)
							return
						}
						packet := make([]byte, len)
						n, err := replaycon.Read(packet)
						if err != nil || n != int(len) {
							fmt.Fprintln(md.log, "unable to read replay packet, expected", len, "received", n, err)
							return
						}
						md.processPacket(packet)
					}
				}()
			}

			err = binary.Write(replaycon, binary.LittleEndian, request)
			if err != nil {
				fmt.Fprintln(md.log, "unable to write replay request ", err)
				replaycon.Close()
				replaycon = nil
			}
		}
	}()
}

func (c *marketDataReceiver) packetReceived(expected uint64, buf []byte) uint64 {
	pn := binary.LittleEndian.Uint64(buf)
	if pn < expected {
		// server restart, reset the packet numbers
		expected = 0
		lastSequence = make(map[Instrument]uint64)
	}

	if expected != 0 && pn != expected {
		// dropped some packets
		request := protocol.ReplayRequest{Start: expected, End: pn}
		fmt.Fprintln(c.log, "dropped packets from", expected, "to", pn)
		replayRequests <- request
	}

	c.processPacket(buf)
	return pn + 1
}

var lastSequence = make(map[Instrument]uint64)
var seqLock = sync.Mutex{}

func (c *marketDataReceiver) processPacket(packet []byte) {
	seqLock.Lock() // need locking because the main md go routine and the replay go routine call through here
	defer seqLock.Unlock()

	packet = packet[8:] // skip over packet number

	buf := bytes.NewBuffer(packet)

	for buf.Len() > 0 {
		book, trades := protocol.DecodeMarketEvent(buf)
		if book != nil {
			last, ok := lastSequence[book.Instrument]
			if (ok && book.Sequence > last) || !ok {
				c.callback.OnBook(book)
				lastSequence[book.Instrument] = book.Sequence
			}
		}
		for _, trade := range trades {
			c.callback.OnTrade(&trade)
		}
	}

}
