package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/quickfixgo/quickfix"
	"github.com/robaho/go-trader/internal/exchange"
	"github.com/robaho/go-trader/pkg/common"
)

import _ "net/http/pprof"

func main() {

	runtime.GOMAXPROCS(8)

	fix := flag.String("fix", "configs/qf_got_settings", "set the fix session file")
	port := flag.String("port", "8080", "set the web server port")
	profile := flag.Bool("profile", false, "create CPU profiling output")

	flag.Parse()

	cfg, err := os.Open(*fix)
	if err != nil {
		panic(err)
	}
	appSettings, err := quickfix.ParseSettings(cfg)
	if err != nil {
		panic(err)
	}
	storeFactory := quickfix.NewMemoryStoreFactory()
	//logFactory, _ := quickfix.NewFileLogFactory(appSettings)
	useLogging, err := appSettings.GlobalSettings().BoolSetting("Logging")
	var logFactory quickfix.LogFactory
	if useLogging {
		logFactory = quickfix.NewScreenLogFactory()
	} else {
		logFactory = quickfix.NewNullLogFactory()
	}
	acceptor, err := quickfix.NewAcceptor(&exchange.App, storeFactory, appSettings, logFactory)
	if err != nil {
		panic(err)
	}

	var ex = &exchange.TheExchange

	ex.Start()

	_ = acceptor.Start()
	defer acceptor.Stop()

	exchange.StartWebServer(":" + *port)
	fmt.Println("web server access available at :" + *port)

	if *profile {
		runtime.SetBlockProfileRate(1)
	}

	watching := sync.Map{}

	fmt.Println("use 'help' to get a list of commands")
	fmt.Print("Command?")

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		s := scanner.Text()
		parts := strings.Fields(s)
		if len(parts) == 0 {
			goto again
		}
		if "help" == parts[0] {
			fmt.Println("The available commands are: quit, sessions, book SYMBOL, watch SYMBOL, unwatch SYMBOL")
		} else if "quit" == parts[0] {
			break
		} else if "sessions" == parts[0] {
			fmt.Println("Active sessions: ", ex.ListSessions())
		} else if "book" == parts[0] {
			book := exchange.GetBook(parts[1])
			if book != nil {
				fmt.Println(book)
			}
		} else if "watch" == parts[0] && len(parts) == 2 {
			fmt.Println("You are now watching ", parts[1], ", use 'unwatch ", parts[1], "' to stop.")
			watching.Store(parts[1], "watching")
			go func(symbol string) {
				var lastBook *common.Book = nil
				for {
					if _, ok := watching.Load(symbol); !ok {
						break
					}
					book := exchange.GetBook(symbol)
					if book != nil {
						if lastBook != book {
							fmt.Println(book)
							lastBook = book
						}
					}
					time.Sleep(1 * time.Second)
				}
			}(parts[1])
		} else if "unwatch" == parts[0] && len(parts) == 2 {
			watching.Delete(parts[1])
			fmt.Println("You are no longer watching ", parts[1])
		} else {
			fmt.Println("Unknown command, '", s, "' use 'help'")
		}
	again:
		fmt.Print("Command?")
	}
}
