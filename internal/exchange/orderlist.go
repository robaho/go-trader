package exchange

import (
	"fmt"
	"strings"

	"github.com/robaho/go-trader/pkg/common"
)

type listNode struct {
	prev *listNode
	next *listNode;
	order sessionOrder
}

// optimized structure to allow efficient removal at start, middle, and end
type orderList struct {
	head *listNode
	tail *listNode
	size int
	allOrders map[sessionOrder]*listNode
}

func OrderList() orderList {
	return orderList{allOrders: make(map[sessionOrder]*listNode)}
}

func (list *orderList) String() string {
	var sb strings.Builder
	for node := list.head; node!=nil; node=node.next {
		if node!=list.head {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprint(node.order.order.Id,":",node.order.order.Remaining))
	}
	return sb.String()
}

func (l *orderList) top() sessionOrder {
	return l.head.order
}

func (l *orderList) pushBack(so sessionOrder) {
	node := &listNode{prev: l.tail, next: nil, order: so}
	if(l.tail!=nil) {
		l.tail.next = node
	}
	if(l.head==nil) {
		l.head = node
	}
	l.tail = node
	l.size++
	l.allOrders[so]=node
}

func (l *orderList) pushFront(so sessionOrder) {
	node := &listNode{prev: nil, next: l.head, order: so}
	if l.head!=nil {
		l.head.prev = node
	}
	l.head = node
	l.size++
	l.allOrders[so]=node
}
func (l *orderList) remove(so sessionOrder) error {
	node,ok := l.allOrders[so]
	if !ok {
		return common.OrderNotFound
	}
	delete(l.allOrders,so)

	if node == l.head {
		if(node.next!=nil) {
			node.next.prev=nil
		}
		l.head = node.next;
	} else if node == l.tail {
		l.tail = node.prev
		if node.prev!=nil {
			node.prev.next=nil
		}
	} else {
		node.prev.next = node.next
		node.next.prev = node.prev
	}
	node.next=nil
	node.prev=nil
	l.size--
	return nil
}

