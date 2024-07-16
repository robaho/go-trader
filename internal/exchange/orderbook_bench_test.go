package exchange

import (
	"fmt"
	"testing"
	"time"

	. "github.com/robaho/fixed"
	. "github.com/robaho/go-trader/pkg/common"
)

func BenchmarkOrders(b *testing.B) {
	var ob = orderBook{}
	var inst = Equity{}
	var ex = testExchangeClient{}

	const N_ORDERS = 1000000

	b.ResetTimer()

	b.N = N_ORDERS*2

	for i:=0;i<N_ORDERS;i++ {
		var o1 = LimitOrder(inst, Buy, NewDecimal("100"), NewDecimal("10"))
		o1.ExchangeId = fmt.Sprint(i)
		var s1 = sessionOrder{ex, o1, time.Now()}
		ob.add(s1)
	}
	for i:=0;i<N_ORDERS;i++ {
		var o1 = LimitOrder(inst, Sell, NewDecimal("100"), NewDecimal("10"))
		o1.ExchangeId = "S"+fmt.Sprint(i)
		var s1 = sessionOrder{ex, o1, time.Now()}
		ob.add(s1)
	}

	b.Log("time per op",float64(b.Elapsed().Microseconds())/float64(N_ORDERS*2))

}

func BenchmarkMultiLevel(b *testing.B) {
	var ob = orderBook{}
	var inst = Equity{}
	var ex = testExchangeClient{}

	const N_ORDERS = 1000000

	b.ResetTimer()

	b.N = N_ORDERS*2

	for i:=0;i<N_ORDERS;i++ {
		var o1 = LimitOrder(inst, Buy, NewF(float64(100+1*(i%1000))), NewDecimal("10"))
		o1.ExchangeId = fmt.Sprint(i)
		var s1 = sessionOrder{ex, o1, time.Now()}
		ob.add(s1)
	}
	for i:=0;i<N_ORDERS;i++ {
		var o1 = LimitOrder(inst, Sell, NewF(float64(100+1*(i%1000))), NewDecimal("10"))
		o1.ExchangeId = "S"+fmt.Sprint(i)
		var s1 = sessionOrder{ex, o1, time.Now()}
		ob.add(s1)
	}

	b.Log("multi-level time per op",float64(b.Elapsed().Microseconds())/float64(N_ORDERS*2))

}