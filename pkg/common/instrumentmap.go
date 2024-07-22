package common

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// global instrument map which is fully synchronized
var IMap instrumentMap

type instrumentMap struct {
	id       int64
	bySymbol sync.Map
	byID     sync.Map
}

func (im *instrumentMap) GetBySymbol(symbol string) Instrument {

	i, ok := im.bySymbol.Load(symbol)
	if !ok {
		return nil
	}
	return i.(Instrument)
}
func (im *instrumentMap) GetByID(id int64) Instrument {
	i, ok := im.byID.Load(id)
	if !ok {
		return nil
	}
	return i.(Instrument)
}
func (im *instrumentMap) AllSymbols() []string {
	var symbols []string

	fn := func (key any,value any) bool {
		symbols = append(symbols, key.(string))
		return true
	}
	im.bySymbol.Range(fn)
	return symbols
}

// the put/nextID are only needed if dynamic instrument creation is added, or for test cases

func (im *instrumentMap) NextID() int64 {
	return atomic.AddInt64(&im.id, 1)
}

func (im *instrumentMap) Put(instrument Instrument) {
	im.bySymbol.Store(instrument.Symbol(),instrument)
	im.byID.Store(instrument.ID(), instrument)
}

// load the instrument map from a file, see configs/instruments.txt for the format
func (im *instrumentMap) Load(filepath string) error {
	inputFile, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "//") || strings.HasPrefix(s, "#") {
			continue
		}
		if s == "" {
			continue
		}
		parts := strings.Fields(s)
		id := ParseInt(parts[0])
		if len(parts) == 2 {
			i := NewInstrument(int64(id), parts[1])
			im.Put(i)
		}
	}
	return nil
}

func init() {
}
