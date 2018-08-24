package common

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var IMap instrumentMap

type instrumentMap struct {
	sync.RWMutex
	id       int64
	bySymbol map[string]Instrument
	byID     map[int64]Instrument
}

func (im *instrumentMap) GetBySymbol(symbol string) Instrument {
	im.RLock()
	defer im.RUnlock()

	i, ok := im.bySymbol[symbol]
	if !ok {
		return nil
	}
	return i
}
func (im *instrumentMap) GetByID(id int64) Instrument {
	im.RLock()
	defer im.RUnlock()

	i, ok := im.byID[id]
	if !ok {
		return nil
	}
	return i
}
func (im *instrumentMap) AllSymbols() []string {
	im.RLock()
	defer im.RUnlock()

	var symbols []string
	for k, _ := range im.bySymbol {
		symbols = append(symbols, k)
	}
	return symbols
}

// the put/nextID are only needed if dynamic instrument creation is added, or for test cases

func (im *instrumentMap) nextID() int64 {
	return atomic.AddInt64(&im.id, 1)
}
func (im *instrumentMap) Put(instrument Instrument) {
	im.Lock()
	defer im.Unlock()

	im.bySymbol[instrument.Symbol()] = instrument
	im.byID[instrument.ID()] = instrument
}

func init() {
	IMap.bySymbol = make(map[string]Instrument)
	IMap.byID = make(map[int64]Instrument)

	inputFile, err := os.Open("instruments.txt")
	if err != nil {
		return
	}
	defer inputFile.Close()

	scanner := bufio.NewScanner(inputFile)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "//") {
			continue
		}
		parts := strings.Fields(s)
		id := ParseInt(parts[0])
		if parts[1] == "E" {
			i := NewEquity(int64(id), parts[2])
			IMap.Put(i)
		}
	}
}
