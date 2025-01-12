package main

import (
	"fmt"
	"sort"
	"time"
)

type Match struct {
	Ask        *Order
	Bid        *Order
	SizeFilled float64
	Price      float64
}

type Order struct {
	Size      float64
	Bid       bool
	Limit     *Limit
	Timestamp int64
}

type Orders []*Order

func (o Orders) Len() int           { return len(o) }
func (o Orders) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }
func (o Orders) Less(i, j int) bool { return o[i].Timestamp < o[j].Timestamp }

func NewOrder(bid bool, size float64) *Order {
	return &Order{
		Size:      size,
		Bid:       bid,
		Timestamp: time.Now().UnixNano(),
	}
}

func (o *Order) String() string {
	return fmt.Sprintf("[size: %.2f]", o.Size)
}

// group of order on certain price level
type Limit struct {
	Price       float64
	Orders      Orders
	TotalVolume float64
}

type Limits []*Limit

type ByBestAsk struct{ Limits }

func (a ByBestAsk) Len() int           { return len(a.Limits) }
func (a ByBestAsk) Swap(i, j int)      { a.Limits[i], a.Limits[j] = a.Limits[j], a.Limits[i] }
func (a ByBestAsk) Less(i, j int) bool { return a.Limits[i].Price < a.Limits[j].Price }

type ByBestBid struct{ Limits }

func (b ByBestBid) Len() int           { return len(b.Limits) }
func (b ByBestBid) Swap(i, j int)      { b.Limits[i], b.Limits[j] = b.Limits[j], b.Limits[i] }
func (b ByBestBid) Less(i, j int) bool { return b.Limits[i].Price > b.Limits[j].Price }

func NewLimit(price float64) *Limit {
	return &Limit{
		Price:  price,
		Orders: []*Order{},
	}
}

func (l *Limit) String() string {
	return fmt.Sprintf("[price: %.2f] | volume: %.2f", l.Price, l.TotalVolume)
}

// limit is bucket of orders
// of different sizes but sitting on the same
// price lvl
func (l *Limit) AddOrder(o *Order) {
	o.Limit = l
	l.Orders = append(l.Orders, o)
	l.TotalVolume += o.Size
}

func (l *Limit) DeleteOrder(o *Order) {
	for i := 0; i < len(l.Orders); i++ {
		if l.Orders[i] == o {
			l.Orders[i] = l.Orders[len(l.Orders)-1]
			l.Orders = l.Orders[:len(l.Orders)-1]
		}
	}
	o.Limit = nil
	l.TotalVolume -= o.Size

	sort.Sort(l.Orders)
}

type OrderBook struct {
	Asks []*Limit // selling
	Bids []*Limit // buying

	AsksLimits map[float64]*Limit
	BidsLimits map[float64]*Limit
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		Asks:       []*Limit{},
		Bids:       []*Limit{},
		AsksLimits: make(map[float64]*Limit),
		BidsLimits: make(map[float64]*Limit),
	}
}

func (ob *OrderBook) PlaceOrder(price float64, o *Order) []Match {
	// 1. Try to match the order
	// matching logic

	// 2. add the rest to the order to the books

	// if order is completely filled size == 0
	if o.Size > 0.0 {
		ob.add(price, o)

	}

	return []Match{}

}

// BUY 15BTC => 100K
func (ob *OrderBook) add(price float64, o *Order) {
	var limit *Limit

	if o.Bid {
		limit = ob.BidsLimits[price]
	} else {
		limit = ob.AsksLimits[price]
	}

	if limit == nil {
		limit = NewLimit(price)
		limit.AddOrder(o)

		if o.Bid {
			ob.Bids = append(ob.Bids, limit)
			ob.BidsLimits[price] = limit
		} else {
			ob.Asks = append(ob.Asks, limit)
			ob.AsksLimits[price] = limit
		}
	}

}