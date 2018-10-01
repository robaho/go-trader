var sequence
var symbol

function connect() {
    var serverUrl = "ws://" + window.location.hostname + ":6502";

    connection = new WebSocket(serverUrl);
    connection.onopen = function () {
        send();
    }

    connection.onmessage = function(evt) {

        var f = document.getElementById("book");
        if (evt.data!=null) {
            var book = JSON.parse(evt.data)
            f.innerHTML = buildBookHtml(book)
            sequence = book.Sequence
        }
    }

    connection.onerror = function (evt) {
        var f = document.getElementById("book")
        f.innerText = "an error occurred"
        console.error("WebSocket error observed:", evt);
    }

    connection.onclose = function (evt) {
        var f = document.getElementById("book")
        f.innerText = "web socket is closed"
        console.error("WebSocket closed:", evt);
    }
}

function send() {
    var msg = {
        symbol: symbol,
        sequence: sequence
    };
    connection.send(JSON.stringify(msg));
}

function buildBookHtml(book) {
    var s = "<table class='book' width='100%' height='100%'>"
    s = s + "<tr><th class='bidqty'/><th class='price'/><th class='askqty'/></tr>";
    if(book.Asks!=null) {
        for (let ask of book.Asks.reverse()) {
            s = s + `<tr><td class="bidqty bid"></td><td class="price ask">${Number(ask.Price).toFixed(2)}</td><td class="askqty ask">${ask.Quantity}</td></tr>`
        }
    }
    if(book.Bids!=null) {
        for (let bid of book.Bids) {
            s = s + `<tr><td class="bidqty bid">${bid.Quantity}</td><td class="price bid">${Number(bid.Price).toFixed(2)}</td><td class="askqty ask"></td></tr>`
        }
    }
    s = s + "</table>"
    return s

}
