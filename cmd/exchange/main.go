package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/robaho/go-trader/pkg/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
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

	fix := flag.String("fix", "configs/qf_got_settings", "set the fix session file")
	props := flag.String("props", "configs/got_settings", "set the exchange properties file")
	instruments := flag.String("instruments", "configs/instruments.txt", "the instrument file")
	port := flag.String("port", "8080", "set the web server port")
	profile := flag.Bool("profile", false, "create CPU profiling output")

	flag.Parse()

	p, err := common.NewProperties(*props)
	if err != nil {
		fmt.Println("unable to exchange properties", err)
	}

	err = common.IMap.Load(*instruments)
	if err != nil {
		fmt.Println("unable to load instruments", err)
	}

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

	// start grpc protocol

	grpc_port := p.GetString("grpc_port", "5000")

	lis, err := net.Listen("tcp", ":"+grpc_port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	} else {
		log.Println("accepting grpc connections at ", lis.Addr())
	}
	s := grpc.NewServer()
	protocol.RegisterExchangeServer(s, exchange.NewGrpcServer())
	// Register reflection service on gRPC server.
	reflection.Register(s)

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

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
			fmt.Println("The available commands are: quit, sessions, book SYMBOL, watch SYMBOL, unwatch SYMBOL, list")
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
		} else if "list" == parts[0] {
			for _, symbol := range common.IMap.AllSymbols() {
				instrument := common.IMap.GetBySymbol(symbol)
				fmt.Println(instrument)
			}
		} else {
			fmt.Println("Unknown command, '", s, "' use 'help'")
		}
	again:
		fmt.Print("Command?")
	}
}
