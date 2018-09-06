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
            f.innerText = evt.data
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
