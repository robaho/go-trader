package exchange

import (
	"testing"
	"time"

	. "github.com/robaho/fixed"
	. "github.com/robaho/go-trader/pkg/common"
)

type dummyClient struct{ id string }

func (c dummyClient) SendOrderStatus(so sessionOrder) {}
func (c dummyClient) SendTrades(trades []trade)       {}
func (c dummyClient) SessionID() string               { return c.id }

func TestOrderList_PushBack(t *testing.T) {
	list := OrderList()
	client := dummyClient{id: "c1"}
	inst := NewInstrument(1, "AAPL")

	o1 := LimitOrder(inst, Buy, NewS("100"), NewS("10"))
	so1 := sessionOrder{client, o1, time.Now()}

	o2 := LimitOrder(inst, Buy, NewS("100"), NewS("20"))
	so2 := sessionOrder{client, o2, time.Now()}

	list.pushBack(so1)
	if list.size != 1 || list.top().order != o1 {
		t.Fatalf("expected size 1 with top o1")
	}

	list.pushBack(so2)
	if list.size != 2 || list.top().order != o1 {
		t.Fatalf("expected size 2 with top o1 (o2 is at back)")
	}
}

func TestOrderList_PushFront(t *testing.T) {
	list := OrderList()
	client := dummyClient{id: "c1"}
	inst := NewInstrument(1, "AAPL")

	o1 := LimitOrder(inst, Buy, NewS("100"), NewS("10"))
	so1 := sessionOrder{client, o1, time.Now()}

	o2 := LimitOrder(inst, Buy, NewS("100"), NewS("20"))
	so2 := sessionOrder{client, o2, time.Now()}

	list.pushBack(so1)
	list.pushFront(so2) // should go to front

	if list.size != 2 {
		t.Fatalf("expected size 2")
	}
	if list.top().order != o2 {
		t.Fatalf("expected top to be o2 after pushFront")
	}
}

func TestOrderList_Remove(t *testing.T) {
	list := OrderList()
	client := dummyClient{id: "c1"}
	inst := NewInstrument(1, "AAPL")

	o1 := LimitOrder(inst, Buy, NewS("100"), NewS("10"))
	so1 := sessionOrder{client, o1, time.Now()}

	o2 := LimitOrder(inst, Buy, NewS("100"), NewS("20"))
	so2 := sessionOrder{client, o2, time.Now()}

	o3 := LimitOrder(inst, Buy, NewS("100"), NewS("30"))
	so3 := sessionOrder{client, o3, time.Now()}

	// Test removing from empty
	if err := list.remove(so1); err == nil {
		t.Fatalf("expected error removing from empty list")
	}

	list.pushBack(so1)
	list.pushBack(so2)
	list.pushBack(so3)

	// Remove middle (so2)
	if err := list.remove(so2); err != nil {
		t.Fatalf("unexpected error removing middle: %v", err)
	}
	if list.size != 2 {
		t.Fatalf("expected size 2, got %d", list.size)
	}

	// Remove head (so1)
	if err := list.remove(so1); err != nil {
		t.Fatalf("unexpected error removing head: %v", err)
	}
	if list.size != 1 || list.top().order != o3 {
		t.Fatalf("expected size 1 and top o3")
	}

	// Remove tail (which is also head, so3)
	if err := list.remove(so3); err != nil {
		t.Fatalf("unexpected error removing tail: %v", err)
	}
	if list.size != 0 {
		t.Fatalf("expected size 0, got %d", list.size)
	}
	if list.head != nil || list.tail != nil {
		t.Fatalf("expected head and tail to be cleared")
	}
}
