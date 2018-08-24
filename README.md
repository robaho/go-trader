# go-trader

A financial exchange written in Go. Uses quickfixgo for client/server communication. Uses UDP multicast for market distribution.

The client offers a command line GUI, "market maker", and a "playback".

The exchange itself has a bare bones web access component with enhancement plans.

The exchange is designed to allow for easy back-testing of trading strategies.

It was primarly developed to further my knowledge of Go and test its suitability for high-performance financial applications.

# install

git clone git://github.com/robaho/go-trader

# build

cd go-trader

export GOPATH=$GOPATH:~/go-trader

go install exchange

go install client

go install marketmaker

go install playback

# run

cd bin

./exchange &

./marketmaker -symbol IBM

./client

# planned TODOs

Finish the /symbol exchange web access to provide a live book depth using web sockets and JSON