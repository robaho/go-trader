package exchange

import (
	"github.com/gernest/hot"
	"github.com/robaho/go-trader/pkg/common"
	"golang.org/x/net/websocket"
	"net/http"
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
		http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("web/assets/js"))))
		http.HandleFunc("/book", bookHandler)
		http.HandleFunc("/instruments", instrumentsHandler)
		http.HandleFunc("/sessions", sessionsHandler)
		http.HandleFunc("/", welcomeHandler)
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
