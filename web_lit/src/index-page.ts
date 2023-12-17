import { LitElement, PropertyValueMap, html } from 'lit'
import { customElement, state } from 'lit/decorators.js'
import { repeat } from 'lit/directives/repeat.js';
import './book-element';
import '@shoelace-style/shoelace';

@customElement('index-page')
export class IndexPage extends LitElement {

    @state()
    instruments: string[] = [];

    @state()
    books: string[] = [];

    protected firstUpdated(_changedProperties: PropertyValueMap<any> | Map<PropertyKey, unknown>): void {
        fetch('/api/instruments').then(response => response.json()).then(x => this.instruments = x.sort());
    }

    maybeAddSymbol(symbol: string) {
        if (!this.books.includes(symbol)) {
            this.books = [symbol, ...this.books]
        }
    }

    render() {
        return html`
            <div style="display: flex; flex-direction: row; height: 100%; width: 100%; gap: 10px">
                <div style="align: top; display: flex; flex-direction: column">
                    <div><div>Instruments</div><hr></div>
                    ${this.instruments.map(b => html`
                        <sl-tooltip placement="right">
                            <div slot="content" style="font-size: 8px">Click to show book</div>
                            <sl-button variant="text" size="medium" @click=${() => this.maybeAddSymbol(b)}>${b}</sl-button>
                        </sl-tooltip>`)}
                </div>
                <div >
                    <div style="display:flex; flex-direction: row; gap: 5px; flex-wrap: wrap">
                        ${repeat(this.books, (key: string) => key, (b: string) => html`<book-element @closed="${() => this.books = this.books.filter(e => e != b)}" symbol="${b}"></book-element>
                    </div>
                </div>
        </div>`)}`;
    }
}
