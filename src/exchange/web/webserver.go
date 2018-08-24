package web

import (
	"common"
	"exchange/internal"
	"html/template"
	"net/http"
	"path/filepath"
)

type empty struct{}

var templatePath = "exchange/web/html/"
var exchange = &internal.TheExchange

func StartWebServer(addr string) {
	go func() {
		http.HandleFunc("/instruments", instrumentsHandler)
		http.HandleFunc("/sessions", sessionsHandler)
		http.HandleFunc("/", welcomeHandler)
		http.ListenAndServe(addr, nil)
	}()
}

func welcomeHandler(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles(filepath.Join(templatePath, "welcome.html"))
	t.Execute(w, empty{})
}

func sessionsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]string)
	data["Sessions"] = exchange.ListSessions()

	t, _ := template.ParseFiles(filepath.Join(templatePath, "sessions.html"))
	t.Execute(w, data)
}
func instrumentsHandler(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]interface{})
	data["Symbols"] = common.IMap.AllSymbols()

	t, _ := template.ParseFiles(filepath.Join(templatePath, "instruments.html"))
	t.Execute(w, data)
}
