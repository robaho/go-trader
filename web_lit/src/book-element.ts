import { LitElement, PropertyValueMap, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import '@shoelace-style/shoelace';

type BookEntry = {
    Price: number,
    Quantity: number
};

type Book = {
    Asks: BookEntry[],
    Bids: BookEntry[]
};

@customElement('book-element')
export class BookElement extends LitElement {

    static styles = [css`
    .bid {
        color: chartreuse;
        text-align: left;
    }
    .ask {
        color: crimson;
        text-align: right;
    }
    div.book {
        background-color: black;
        height: 100%;
        font-size: large;
    }
    table.book {
        padding: 0 0 0 0;
    }
    .askqty {
        width: 30%;
        text-align: right;
    }
    .bidqty {
        width: 30%;
        text-align: right;
    }
    .price {
        width: 40%;
        text-align: right;
        background-color: rgba(170, 170, 170, 0.39);
    }
    body {
        background-color: black;
        color: white;
    }`];

    @property()
    symbol !: string;

    @state()
    error?: string;

    @state()
    book: Book = { Asks: [], Bids: [] };

    connection?: WebSocket;

    private subscribe() {
        this.book = { Asks: [], Bids: [] };
        var msg = {
            symbol: this.symbol
        };
        this.connection?.send(JSON.stringify(msg));
    }

    private connect() {
        var serverUrl = "ws://" + window.location.hostname + ":6502";

        if (this.connection) {
            this.subscribe();
            return;
        }

        this.connection = new WebSocket(serverUrl);

        this.connection.onopen = () => {
            this.error = undefined;
            this.subscribe();
        }

        this.connection.onmessage = (evt) => {
            if (evt.data != null) {
                var book = JSON.parse(evt.data);
                this.book = book;
            }
        }

        this.connection.onerror = (evt) => {
            this.error = "an error occurred " + evt
        }

        this.connection.onclose = () => {
            this.error = "web socket is closed"
        }
    }

    disconnectedCallback(): void {
        super.disconnectedCallback();
        this.connection?.close();
        this.connection = undefined;
    }

    protected update(changedProperties: PropertyValueMap<any> | Map<PropertyKey, unknown>): void {
        if (changedProperties.has('symbol')) {
            this.connect();
        }
        super.update(changedProperties);
    }

    private close() {
        this.connection?.close();
        this.connection = undefined;
        this.dispatchEvent(new Event('closed'));
    }

    render() {
        return html`
            <div style="width: 200px; height: 300px; background: black">
                <div style="display:flex; flex-direction: row; align-items: center; gap: 10px"><span>Order Book : ${this.symbol}</span><sl-icon-button style="margin-left:auto" size="small" name="x-circle" @click=${() => this.close()}></sl-icon-button></div>
                <hr>
                ${this.error ? html`<h4>${this.error}</h4>` : html`
                <table class='book' width='100%' max-height='100%'>
                    <tr><th class='bidqty'></th><th class='price'></th><th class='askqty'></th></tr>
                    ${this.book.Asks?.reverse().map(ask => html`<tr><td class="bidqty bid"></td><td class="price ask">${Number(ask.Price).toFixed(2)}</td><td class="askqty ask">${ask.Quantity}</td></tr>`)}
                    ${this.book.Bids?.map(bid => html`<tr><td class="bidqty bid">${bid.Quantity}</td><td class="price bid">${Number(bid.Price).toFixed(2)}</td><td class="askqty ask"></td></tr>`)}
                </table>`}
            </div>`;
    }
}