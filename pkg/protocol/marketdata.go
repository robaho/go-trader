package protocol

import (
	"bytes"
	. "github.com/robaho/go-trader/pkg/common"
	"io"
)

// very simplified structure, only one book and associated trades per UDP packet, and it contains the complete book
// currently it all needs to fit in a single packet or it won't work, although straightforward to send additional trade packets

func EncodeMarketEvent(book *Book, trades []Trade) []byte {
	buf := new(bytes.Buffer)
	if book != nil {
		buf.WriteByte(1) // has book
		buf.Write(encodeBook(book))
	} else {
		buf.WriteByte(0) // no book
	}
	buf.Write(encodeTrades(trades))
	return buf.Bytes()
}

func DecodeMarketEvent(r io.ByteReader) (*Book, []Trade) {
	hasBook, _ := r.ReadByte()
	var book *Book
	if hasBook == 1 {
		book = decodeVook(r)
	}
	trades := decodeTrades(r)
	return book, trades
}

func encodeBook(book *Book) []byte {
	buf := new(bytes.Buffer)

	PutVarint(buf, book.Instrument.ID())
	PutUvarint(buf, book.Sequence)

	encodeLevels(buf, book.Bids)
	encodeLevels(buf, book.Asks)

	return buf.Bytes()
}

func decodeVook(r io.ByteReader) *Book {
	book := new(Book)

	instrumentId, _ := ReadVarint(r)
	sequence, _ := ReadUvarint(r)

	instrument := IMap.GetByID(instrumentId)

	book.Instrument = instrument
	book.Sequence = sequence

	book.Bids = decodeLevels(r)
	book.Asks = decodeLevels(r)

	return book
}

func encodeLevels(w io.ByteWriter, levels []BookLevel) {
	w.WriteByte(byte(len(levels)))
	for _, level := range levels {
		EncodeDecimal(w, level.Price)
		EncodeDecimal(w, level.Quantity)
	}
}

func decodeLevels(r io.ByteReader) []BookLevel {
	n, _ := r.ReadByte()
	levels := make([]BookLevel, n)
	for i := 0; i < int(n); i++ {
		price := DecodeDecimal(r)
		qty := DecodeDecimal(r)
		levels[i] = BookLevel{Price: price, Quantity: qty}
	}
	return levels
}

// this will blow up if any given match generates a ton of trades...
func encodeTrades(trades []Trade) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte(byte(len(trades)))
	for _, v := range trades {
		PutVarint(buf, v.Instrument.ID())
		EncodeDecimal(buf, v.Quantity)
		EncodeDecimal(buf, v.Price)
		EncodeString(buf, v.ExchangeID)
		EncodeTime(buf, v.TradeTime)
	}

	return buf.Bytes()
}

func decodeTrades(r io.ByteReader) []Trade {
	n, _ := r.ReadByte() // read length
	trades := make([]Trade, n)
	for i := 0; i < int(n); i++ {
		instrumentId, _ := ReadVarint(r)
		instrument := IMap.GetByID(instrumentId)

		qty := DecodeDecimal(r)
		price := DecodeDecimal(r)
		exchangeID := DecodeString(r)
		tradeTime := DecodeTime(r)

		trades[i] = Trade{Instrument: instrument, Price: price, Quantity: qty, ExchangeID: exchangeID, TradeTime: tradeTime}
	}
	return trades

}

type ReplayRequest struct {
	// Start is inclusive, and End is exclusive
	Start, End uint64
}
