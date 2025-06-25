# go-trader

A financial exchange written in Go including complete order book, fix protocol, and market data distribution.

Check out [cpp-trader](https://github.com/robaho/cpp-trader) for a C++ version, albeit with less features.

## Features

- client to server communication using:
    - FIX (using quickfixgo)
    - gRPC
- UDP multicast for market data distribution.
- TCP replay of dropped market data packets.
- Uses the high-performance fixed point library [fixed](https://github.com/robaho/fixed) which I also developed.
- Includes multiple clients:
    - command line 
    - server-side web using Go templates
    - SPA web using Lit
- A sample "market maker" to mass quote the market.
- A sample "playback" to simulate markets from recorded market data.
- Supported order types:
    - limit
    - market
- [REST api](https://github.com/robaho/go-trader/blob/2b92b5652eb5c6a93b83262f45ba1f237fb180b0/internal/exchange/webserver.go#L41-L54)

The exchange is designed to allow for easy back-testing of trading strategies. It supports limit and market orders.

There is a very simple sample "algo". The program structure is applicable to many strategies that use an entry and exit price.
This can be run in conjunction with the 'marketmaker' sample to test the "algo". Hint: it has a 50/50 chance of being successful EXCEPT the
market maker bid/ask spread must be accounted for - which makes it far less than a 50/50 chance of being profitable...

There are two different web interfaces available:
- the default interface at `/` uses Go templates and server side rendering
- the alternative UI at `/lit` is written in Typescript using [Lit](https://lit.dev) and the Rest api

Use `npm run build` in the `web_lit` directory to build the Lit assets.

It was originally developed to further my knowledge of Go and test its suitability for high-performance financial applications, and it has continued to evolve into an ideal teaching project for Go facilities (interfaces, networking, web development, Go routines, etc.).

# install

`go get github.com/robaho/go-trader`

# build

```
cd go-trader
mkdir bin
go build -o bin ./cmd/...
```

# run

<pre>
cd go-trader

<strong>Run each in a different terminal session.</strong>
<strong>Ensure the correct interface is set in configs/got_settings for the client.</strong>

bin/exchange

bin/marketmaker -symbol IBM

bin/client
</pre>

# performance

Configuration:

- client machine is a Mac Mini M1, running OSX Sonoma
- server machine is a 4.0 ghz i7 iMac (4 core, 8 thread), running OSX Monterey
- using 1 gbit ethernet connection
- a quote is a double-sided (bid & ask) 
- timings are measured from the quote message generation on the client, to the reception of the multicast market data on the client
- 90k+ quote per second over the network using FIX with latency < 1ms
- 400k+ quote per second over the network using gRPC with latency < 600 usec

<details>
    <summary>performance details</summary>
<br>

**using `marketmaker -bench 75 -proto fix`**

```
updates per second 72707, max ups 72707,  avg rtt 832us, 10% rtt 595us 99% rtt 5365us
updates per second 90279, max ups 90279,  avg rtt 830us, 10% rtt 0us 99% rtt 4515us
updates per second 89215, max ups 90279,  avg rtt 840us, 10% rtt 0us 99% rtt 4851us
```

**using `marketmaker -bench 250 -proto grpc`**

```
updates per second 410094, max ups 414584,  avg rtt 609us, 10% rtt 0us 99% rtt 2390us
updates per second 411559, max ups 414584,  avg rtt 607us, 10% rtt 0us 99% rtt 2455us
updates per second 412884, max ups 414584,  avg rtt 605us, 10% rtt 0us 99% rtt 2270us
```

_The CPUs are saturated on both the client and server._

## less than 3 microseconds per roundtrip quote over the network ! ##
<br>
</details>
<br>

# REST api

access full book (use guest/password to login)

localhost:8080/api/book/SYMBOL

localhost:8080/api/stats/SYMBOL

# screen shots

![client screen shot](doc/clientss.png)
![web screen shot](doc/webss.png)
![lit screen shot](doc/litss.png)
