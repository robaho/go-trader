# go-trader

A financial exchange written in Go. Uses quickfixgo for client/server communication. Uses UDP multicast for market distribution.

The client offers a command line GUI. The exchange has web access.

# build

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