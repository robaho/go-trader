package exchange

import (
	"encoding/base64"
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
		http.HandleFunc("/api/book/", authenticate(apiBookHandler))
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

func authenticate(handler func(w http.ResponseWriter, r *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(s) != 2 {
			http.Error(w, "Not authorized", 401)
			return
		}

		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}

		pair := strings.SplitN(string(b), ":", 2)
		if len(pair) != 2 {
			http.Error(w, "Not authorized", 401)
			return
		}

		if pair[0] != "guest" || pair[1] != "password" {
			http.Error(w, "Not authorized", 401)
			return
		}

		handler(w, r)
	}

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
