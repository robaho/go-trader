function connect() {
    var serverUrl = "ws://" + window.location.hostname + ":6502";

    connection = new WebSocket(serverUrl);
    connection.onopen = function () {
        send();
    }

    connection.onmessage = function(evt) {
        var f = document.getElementById("book");
        if (evt.data!=null) {
            f.innerText = evt.data
        }
    }

    connection.onerror = function (evt) {
        var f = document.getElementById("book")
        f.innerText = "an error occurred"
        console.error("WebSocket error observed:", evt);
    }
}

function send() {
    var msg = {
        symbol: document.getElementById("symbol").innerHTML,
    };
    connection.send(JSON.stringify(msg));
}
