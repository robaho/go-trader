package exchange

import (
	"encoding/json"
	"github.com/gernest/hot"
	"github.com/robaho/go-trader/pkg/common"
	"golang.org/x/net/websocket"
	"net/http"
	"strings"
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
		http.HandleFunc("/api/book/", apiBookHandler)
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
}

type BookRequest struct {
	Symbol   string
	Sequence uint64
}

func BookServer(ws *websocket.Conn) {
	for {
		request := BookRequest{}

		if websocket.JSON.Receive(ws, &request) != nil {
			break
		}

		book := GetBook(request.Symbol)
		if book == nil {
			book = &common.Book{}
		}

		if request.Sequence >= book.Sequence { // book hasn't changed
			continue // ignore
		}

		if websocket.JSON.Send(ws, book) != nil {
			break
		}
	}
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
	data["Symbols"] = common.IMap.AllSymbols()

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

	instrument := common.IMap.GetBySymbol(symbol)
	if instrument == nil {
		http.Error(w, "the symbol "+symbol+" is unknown", http.StatusNotFound)
	} else {
		book := GetBook(symbol)
		if book == nil {
			book = &common.Book{}
		}
		b, err := json.Marshal(book)
		if err != nil {
			r.Response.StatusCode = http.StatusInternalServerError
		} else {
			w.Write(b)
		}
	}
}
