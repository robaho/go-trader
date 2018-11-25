package exchange

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/gernest/hot"
	. "github.com/robaho/go-trader/pkg/common"
	"golang.org/x/net/websocket"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type empty struct{}

var templatePath = "web/templates/"

var t *hot.Template

func StartWebServer(addr string) {
	var err error

	config := &hot.Config{
		Watch:          true,
		BaseName:       "hot",
		Dir:            templatePath,
		FilesExtension: []string{".html"},
	}

	tpl, err := hot.New(config)
	if err != nil {
		panic(err)
	}
	t = tpl

	go func() {
		http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("web/assets"))))
		http.HandleFunc("/book", bookHandler)
		http.HandleFunc("/instruments", instrumentsHandler)
		http.HandleFunc("/sessions", sessionsHandler)
		http.HandleFunc("/api/book/", authenticate(apiBookHandler))
		http.HandleFunc("/api/stats/", authenticate(apiStatsHandler))
		http.HandleFunc("/", welcomeHandler)

		// add REST api
		http.ListenAndServe(addr, nil)
	}()

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/", websocket.Handler(BookServer))
		err := http.ListenAndServe(":6502", mux)
		if err != nil {
			panic("ListenAndServe: " + err.Error())
		}
	}()

	go websocketPublisher()
}

func getString(key string, data string) string {
	regex := key + "=" + "\"(?P<Value>.*?)\""
	p := regexp.MustCompile(regex)
	results := p.FindStringSubmatch(data)
	if len(results) > 1 {
		return results[1]
	}
	return ""
}

var nonceMap = make(map[string]bool)

func getNonce() string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	nonces := hex.EncodeToString(nonce)
	nonceMap[nonces] = true
	return nonces
}

func authenticate(handler func(w http.ResponseWriter, r *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(s) != 2 || s[0] != "Digest" {
			w.Header().Set("WWW-Authenticate", `Digest realm="Restricted",nonce="`+getNonce()+`""`)
			http.Error(w, "Not authorized", 401)
			return
		}

		uri := getString("uri", s[1])
		nonce := getString("nonce", s[1])
		response := getString("response", s[1])

		if _, exists := nonceMap[nonce]; !exists {
			w.Header().Set("WWW-Authenticate", `Digest stale=true,realm="Restricted",nonce="`+getNonce()+`""`)
			http.Error(w, "Not authorized", 401)
			return
		}

		delete(nonceMap, nonce)

		h1 := md5.Sum([]byte("guest:Restricted:password"))
		h2 := md5.Sum([]byte(r.Method + ":" + uri))
		h3 := md5.Sum([]byte(hex.EncodeToString(h1[:]) + ":" + nonce + ":" + hex.EncodeToString(h2[:])))

		expected := hex.EncodeToString(h3[:])

		if expected != response {
			http.Error(w, "Not authorized", 401)
			return
		}

		w.Header().Set("Set-Cookie", "golangrocks")

		handler(w, r)
	}

}

type BookRequest struct {
	Symbol   string
	Sequence uint64
}

var webCons sync.Map

func BookServer(ws *websocket.Conn) {
	defer webCons.Delete(ws)

	for {
		request := BookRequest{}

		if websocket.JSON.Receive(ws, &request) != nil {
			break
		}

		webCons.Store(ws, request.Symbol)

		book := GetBook(request.Symbol)
		if book == nil {
			book = &Book{}
		}

		if request.Sequence >= book.Sequence { // book hasn't changed
			continue // ignore
		}

		if websocket.JSON.Send(ws, book) != nil {
			break
		}
	}
}

// publish book updates to subscribed websockets
// no need to subscribe to internal listener, just publish on an interval
func websocketPublisher() {
	var latest = make(map[string]uint64) // track latest sequence number, no need to send anything that hasn't changed
	for {
		// cache json so we only generate once per loop
		var json = make(map[string][]byte)

		webCons.Range(func(key, value interface{}) bool {
			con := key.(*websocket.Conn)
			symbol := value.(string)

			book := GetBook(symbol)
			if book == nil || book.Sequence == latest[symbol] {
				return true
			}
			latest[symbol] = book.Sequence
			msg := json[symbol]
			if msg == nil {
				msg = bookToJSON(symbol, book)
				json[symbol] = msg
			}
			con.Write(msg)
			return true
		})
		time.Sleep(time.Second)
	}
}

func bookToJSON(symbol string, book *Book) []byte {
	m := make(map[string]interface{})
	m["Symbol"] = symbol
	m["Bids"] = book.Bids
	m["Asks"] = book.Asks
	m["Sequence"] = book.Sequence
	msg, _, _ := websocket.JSON.Marshal(m)
	return msg
}

func statsToJSON(symbol string, stats *Statistics) []byte {
	msg, _, _ := websocket.JSON.Marshal(*stats)
	return msg
}

func welcomeHandler(w http.ResponseWriter, r *http.Request) {
	t.Execute(w, "welcome.html", empty{})
}

func sessionsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]string)
	data["Sessions"] = TheExchange.ListSessions()

	t.Execute(w, "sessions.html", data)
}

func instrumentsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]interface{})

	stats := make([]Statistics, 0)

	for _, s := range IMap.AllSymbols() {
		stats0 := getStatistics(IMap.GetBySymbol(s))
		if stats0 == nil {
			s0 := Statistics{}
			s0.Symbol = s
			stats = append(stats, s0)
			continue
		}
		stats = append(stats, *stats0)
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Symbol < stats[j].Symbol
	})

	data["Stats"] = stats

	t.Execute(w, "instruments.html", data)
}

func bookHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()

	symbol := queryValues.Get("symbol")
	data := make(map[string]interface{})
	data["symbol"] = symbol

	t.Execute(w, "book.html", data)
}

func apiBookHandler(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimPrefix(r.URL.Path, "/api/book/")

	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		http.Error(w, "the symbol "+symbol+" is unknown", http.StatusNotFound)
	} else {
		book := GetBook(symbol)
		if book == nil {
			book = &Book{}
		}
		b := bookToJSON(symbol, book)
		w.Write(b)
	}
}

func apiStatsHandler(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimPrefix(r.URL.Path, "/api/stats/")

	instrument := IMap.GetBySymbol(symbol)
	if instrument == nil {
		http.Error(w, "the symbol "+symbol+" is unknown", http.StatusNotFound)
	} else {
		stats := getStatistics(instrument)
		if stats == nil {
			stats = &Statistics{}
		}
		s := statsToJSON(symbol, stats)
		w.Write(s)
	}
}
