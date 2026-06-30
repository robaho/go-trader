package exchange

import (
	"testing"

	. "github.com/robaho/fixed"
	. "github.com/robaho/go-trader/pkg/common"
)

func init() {
	eventChannel = make(chan MarketEvent, 1024)
	lastSentBook = make(map[string]uint64)
}

type mockExchangeClient struct {
	id         string
	ordersSent []sessionOrder
	tradesSent [][]trade
}

func (c *mockExchangeClient) SendOrderStatus(so sessionOrder) {
	c.ordersSent = append(c.ordersSent, so)
}

func (c *mockExchangeClient) SendTrades(trades []trade) {
	c.tradesSent = append(c.tradesSent, trades)
}

func (c *mockExchangeClient) SessionID() string {
	return c.id
}

func TestExchangeLevelBasics(t *testing.T) {
	ex := &exchange{}
	client1 := &mockExchangeClient{id: "client1"}
	client2 := &mockExchangeClient{id: "client2"}

	inst := NewInstrument(1, "AAPL")

	// 1. Submit an order
	order1 := LimitOrder(inst, Buy, NewS("150.00"), NewS("100"))
	order1.Id = 1
	oid1, err := ex.CreateOrder(client1, order1)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	if oid1 != 1 {
		t.Errorf("expected order ID 1, got %v", oid1)
	}

	if order1.OrderState != Booked {
		t.Errorf("expected order state Booked, got %v", order1.OrderState)
	}

	// 2. Modify an order
	err = ex.ModifyOrder(client1, oid1, NewS("151.00"), NewS("100"))
	if err != nil {
		t.Fatalf("ModifyOrder failed: %v", err)
	}

	if order1.Price.String() != "151" {
		t.Errorf("expected modified price 151, got %v", order1.Price)
	}

	// 3. Cross an order to generate a fill
	order2 := LimitOrder(inst, Sell, NewS("150.00"), NewS("50"))
	order2.Id = 2
	_, err = ex.CreateOrder(client2, order2)
	if err != nil {
		t.Fatalf("CreateOrder (crossing) failed: %v", err)
	}

	// order2 should be fully filled
	if order2.OrderState != Filled {
		t.Errorf("expected crossing order to be Filled, got %v", order2.OrderState)
	}

	// order1 should be partially filled
	if order1.OrderState != PartialFill {
		t.Errorf("expected resting order to be PartialFill, got %v", order1.OrderState)
	}
	if order1.Remaining.String() != "50" {
		t.Errorf("expected resting order remaining 50, got %v", order1.Remaining)
	}

	// 4. Try to modify an order that has already been filled
	err = ex.ModifyOrder(client2, order2.Id, NewS("150.00"), NewS("10"))
	if err != OrderIsNotActive {
		t.Errorf("expected OrderIsNotActive when modifying filled order, got %v", err)
	}

	// 5. Cross another order to fully fill the resting order
	order3 := LimitOrder(inst, Sell, NewS("151.00"), NewS("50"))
	order3.Id = 3
	_, err = ex.CreateOrder(client2, order3)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	if order1.OrderState != Filled {
		t.Errorf("expected resting order to be Filled, got %v", order1.OrderState)
	}
	if order3.OrderState != Filled {
		t.Errorf("expected incoming order to be Filled, got %v", order3.OrderState)
	}

	// Verify try to modify order1 now fails / ignores
	err = ex.ModifyOrder(client1, order1.Id, NewS("152.00"), NewS("100"))
	if err != OrderIsNotActive {
		t.Errorf("expected OrderIsNotActive when modifying filled resting order, got %v", err)
	}
}
