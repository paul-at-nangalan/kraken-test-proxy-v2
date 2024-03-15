package orderbooks

import (
	"sort"
	"sync"
)

type BidAsk struct {
	price float64
	qty   float64
}

// /for testing only - ignore qty - for now
type Orderbook struct {
	asks map[float64]float64
	bids map[float64]float64

	bidsdirty   bool
	asksdirty   bool
	orderedbids []BidAsk
	orderedasks []BidAsk

	incomingasks chan *BidAsk
	incomingbids chan *BidAsk

	proclock sync.RWMutex
}

func NewOrderBook() *Orderbook {
	return &Orderbook{
		asks:         make(map[float64]float64),
		bids:         make(map[float64]float64),
		incomingasks: make(chan *BidAsk, 500),
		incomingbids: make(chan *BidAsk, 500),
	}
}

type SharedOrderbook struct {
	books map[string]*Orderbook
}

func NewSharedOrderbook(symbols []string) *SharedOrderbook {
	sob := &SharedOrderbook{
		books: make(map[string]*Orderbook),
	}
	for _, symbol := range symbols {
		sob.books[symbol] = NewOrderBook()
	}
	go sob.Process()
	return sob
}

func (p *SharedOrderbook) GetOrderbook(symbol string) *Orderbook {
	return p.books[symbol]
}

func (p *Orderbook) makeOrderedSlice(data map[float64]float64) []BidAsk {
	ordered := make([]BidAsk, len(data))
	i := 0
	for price, qty := range data {
		ordered[i] = BidAsk{price, qty}
		i++
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].price < ordered[j].price
	})
	return ordered
}

// / Match our incoming bids and asks with the orderbook
func (p *Orderbook) MatchBid(price float64, qty float64) (fillprice float64, fillqty float64) {
	p.proclock.RLock()
	defer p.proclock.RUnlock()
	if p.asksdirty {
		p.orderedasks = p.makeOrderedSlice(p.asks)
		p.asksdirty = false

	}
	/// If our price is lower then the lowest ask, we have no match
	///match on the highest ask price that is less than or equal to our bid price
	for _, ask := range p.orderedasks {
		if ask.price <= price {
			if ask.qty <= qty {
				fillprice = price ///we've bid at a higher price - so fill at that price -
				// should be a worst case scenario and eak out issues with the algo
				fillqty = ask.qty
				return
			} else {
				fillprice = price
				fillqty = qty
				return
			}
		} else {
			break //// our price is lower than the ask, so we can't match (and the list should be ordered)
		}
	}
	return
}

// / NOTE: this is not a perfect matching engine - but should give something reasonable for testing
// /   it does not adjust order book qty's after a match (for now)
// / Match our incoming bids and asks with the orderbook - ask
func (p *Orderbook) MatchAsk(price float64, qty float64) (fillprice float64, fillqty float64) {
	p.proclock.RLock()
	defer p.proclock.RUnlock()
	if p.bidsdirty {
		p.orderedbids = p.makeOrderedSlice(p.bids)
		p.bidsdirty = false
	}

	//// go through in reverse order, match on the lowest bid price that is greater than or equal to our ask price
	for i := 0; i < len(p.bids); i++ {
		bid := p.orderedbids[len(p.bids)-1-i]
		if bid.price >= price {
			if bid.qty <= qty {
				fillprice = price ///we've bid at a higher price - so fill at that price -
				// should be a worst case scenario and eak out issues with the algo
				fillqty = bid.qty
				return
			} else {
				fillprice = price
				fillqty = qty
				return
			}
		} else {
			break //// our price is higher than the bid, so we can't match (and the list should be ordered)
		}
	}
	return
}

func (p *Orderbook) AddBids(bids []*BidAsk) {
	for _, bid := range bids {
		p.incomingbids <- bid
	}
}

func (p *Orderbook) AddAsks(asks []*BidAsk) {
	for _, ask := range asks {
		p.incomingasks <- ask
	}
}

func (p *Orderbook) process() {
	p.proclock.Lock()
	defer p.proclock.Unlock()
	for {
		select {
		case bid := <-p.incomingbids:
			/// go through bids, clear out any that are 0 qty, update or add others
			_, found := p.bids[bid.price]
			if bid.qty == 0 {
				p.bidsdirty = true
				if found {
					delete(p.bids, bid.price)
				}
			} else {
				if !found {
					/// we only need to set the dirty flag if we are adding a new price
					p.bidsdirty = true
				}
				p.bids[bid.price] = bid.qty
			}
		case ask := <-p.incomingasks:
			/// go through asks, clear out any that are 0 qty, update or add others
			_, found := p.asks[ask.price]
			if ask.qty == 0 {
				p.asksdirty = true
				if found {
					delete(p.asks, ask.price)
				}
			} else {
				if !found {
					/// we only need to set the dirty flag if we are adding a new price
					p.asksdirty = true
				}
				p.asks[ask.price] = ask.qty
			}
			p.asksdirty = true
		default:
			return
		}
	}
}

func (p *SharedOrderbook) Process() {
	for _, book := range p.books {
		book.process()
	}
}
