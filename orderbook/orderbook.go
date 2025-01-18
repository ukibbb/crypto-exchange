package orderbook

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Match struct {
	Ask        *Order
	Bid        *Order
	SizeFilled float64
	Price      float64
}

type Order struct {
	ID     int64
	UserID int64
	Size   float64
	Bid    bool
	// limit is a bucket containing orders for the same price of different size.
	Limit     *Limit
	Timestamp int64
}

type Orders []*Order

func (o Orders) Len() int           { return len(o) }
func (o Orders) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }
func (o Orders) Less(i, j int) bool { return o[i].Timestamp < o[j].Timestamp }

func NewOrder(bid bool, size float64, userID int64) *Order {
	return &Order{
		UserID:    userID,
		ID:        int64(rand.Intn(10000000)),
		Size:      size,
		Bid:       bid,
		Timestamp: time.Now().UnixNano(),
	}
}

func (o *Order) String() string {
	return fmt.Sprintf("[size: %.2f]", o.Size)
}

func (o *Order) Type() string {
	if o.Bid {
		return "BID"
	}
	return "ASK"
}

func (o *Order) IsFilled() bool {
	return o.Size == 0.0
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

func (l *Limit) Fill(o *Order) []Match {
	var (
		matches        []Match
		ordersToDelete []*Order
	)

	for _, order := range l.Orders {
		if o.IsFilled() {
			break
		}
		match := l.fillOrder(order, o)
		matches = append(matches, match)

		l.TotalVolume -= match.SizeFilled

		if order.IsFilled() {
			ordersToDelete = append(ordersToDelete, order)
		}

	}
	for _, order := range ordersToDelete {
		l.DeleteOrder(order)
	}
	return matches
}

func (l *Limit) fillOrder(a, b *Order) Match {
	var (
		bid        *Order
		ask        *Order
		sizeFilled float64
	)
	if a.Bid {
		bid = a
		ask = b
	} else {
		bid = b
		ask = a
	}
	if a.Size >= b.Size {
		a.Size -= b.Size
		sizeFilled = b.Size
		b.Size = 0.0
	} else {
		b.Size -= a.Size
		sizeFilled = a.Size
		a.Size = 0.0
	}

	return Match{
		Bid:        bid,
		Ask:        ask,
		SizeFilled: sizeFilled,
		Price:      l.Price,
	}
}

type Trade struct {
	Size      float64
	Price     float64
	Bid       bool
	Timestamp int64
}

type OrderBook struct {
	asks []*Limit // selling
	bids []*Limit // buying

	Trades []*Trade

	mu         sync.RWMutex
	AsksLimits map[float64]*Limit
	BidsLimits map[float64]*Limit

	Orders map[int64]*Order
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		asks:       []*Limit{},
		bids:       []*Limit{},
		Trades:     []*Trade{},
		AsksLimits: make(map[float64]*Limit),
		BidsLimits: make(map[float64]*Limit),
		Orders:     make(map[int64]*Order),
	}
}

func (ob *OrderBook) PlaceMarketOrder(o *Order) []Match {
	matches := []Match{}

	if o.Bid {
		if o.Size > ob.AskTotalVolume() {
			panic(fmt.Errorf("not enough volume [%.2f] for market order [%.2f]", ob.AskTotalVolume(), o.Size))
		}

		for _, limit := range ob.Asks() {
			limitMatches := limit.Fill(o)
			matches = append(matches, limitMatches...)

			if len(limit.Orders) == 0 {
				ob.clearLimit(false, limit)
			}
		}
	} else {
		if o.Size > ob.BidTotalVolume() {
			panic(fmt.Errorf("not enough volume [%.2f] for market order [%.2f]", ob.BidTotalVolume(), o.Size))
		}

		for _, limit := range ob.Bids() {
			limitMatches := limit.Fill(o)
			matches = append(matches, limitMatches...)

			if len(limit.Orders) == 0 {
				ob.clearLimit(true, limit)
			}
		}
	}

	for _, match := range matches {
		trade := &Trade{
			Price:     match.Price,
			Size:      match.SizeFilled,
			Timestamp: time.Now().UnixNano(),
			Bid:       o.Bid,
		}

		ob.Trades = append(ob.Trades, trade)

	}

	return matches
}

// limit orders providing liquidity
func (ob *OrderBook) PlaceLimitOrder(price float64, o *Order) {
	var limit *Limit

	ob.mu.Lock()
	defer ob.mu.Unlock()

	if o.Bid {
		limit = ob.BidsLimits[price]
	} else {
		limit = ob.AsksLimits[price]
	}

	if limit == nil {
		limit = NewLimit(price)
		if o.Bid {
			ob.bids = append(ob.bids, limit)
			ob.BidsLimits[price] = limit
			ob.Orders[o.ID] = o
		} else {
			ob.asks = append(ob.asks, limit)
			ob.AsksLimits[price] = limit
		}
	}
	logrus.WithFields(logrus.Fields{
		"price": limit.Price, // limit is a bucket containning orders of different sizes
		// from different peapole but sitting on the same price lvl
		// and ppl can place ordrers of different sizes of the same limit
		// if sombody will place market order it will eat that limit order by order until limit is gone
		"type":   o.Type(),
		"size":   o.Size,
		"userID": o.UserID,
	}).Info("new limit order")
	ob.Orders[o.ID] = o
	limit.AddOrder(o)
}

func (ob *OrderBook) clearLimit(bid bool, l *Limit) {
	if bid {
		delete(ob.BidsLimits, l.Price)
		for i := 0; i < len(ob.bids); i++ {
			if ob.bids[i] == l {
				ob.bids[i] = ob.bids[len(ob.bids)-1]
				ob.bids = ob.bids[:len(ob.bids)-1]
			}
		}
	} else {
		delete(ob.AsksLimits, l.Price)
		for i := 0; i < len(ob.asks); i++ {
			if ob.asks[i] == l {
				ob.asks[i] = ob.asks[len(ob.asks)-1]
				ob.asks = ob.asks[:len(ob.asks)-1]
			}
		}

	}
}

func (ob *OrderBook) CancelOrder(o *Order) {
	limit := o.Limit
	limit.DeleteOrder(o)
	delete(ob.Orders, o.ID)

	if len(limit.Orders) == 0 {
		ob.clearLimit(o.Bid, limit)
	}

}

func (ob *OrderBook) BidTotalVolume() float64 {
	totalVolume := 0.0

	for i := 0; i < len(ob.bids); i++ {
		totalVolume += ob.bids[i].TotalVolume
	}
	return totalVolume
}
func (ob *OrderBook) AskTotalVolume() float64 {
	totalVolume := 0.0

	for i := 0; i < len(ob.asks); i++ {
		totalVolume += ob.asks[i].TotalVolume
	}
	return totalVolume
}

func (ob *OrderBook) Asks() []*Limit {
	// it's swapping thing in memory
	sort.Sort(ByBestAsk{ob.asks})
	return ob.asks
}

func (ob *OrderBook) Bids() []*Limit {
	// it's swapping thing in memory
	sort.Sort(ByBestBid{ob.bids})
	return ob.bids
}
