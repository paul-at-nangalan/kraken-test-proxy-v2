package orderbooks

import "sync"

type BidAsk struct {
	price float64
	qty   float64
}

// /for testing only - ignore qty - for now
type Orderbook struct {
	asks map[float64]float64
	bids map[float64]float64

	incomingasks chan *BidAsk
	incomingbids chan *BidAsk

	proclock sync.RWMutex
}

type SharedOrderbook struct {
	books map[string]*Orderbook
}

func NewSharedOrderbook(symbols []string) *SharedOrderbook {
	sob := &SharedOrderbook{
		books: make(map[string]*Orderbook),
	}
	for _, symbol := range symbols {
		sob.books[symbol] = &Orderbook{
			asks:         make(map[float64]float64),
			bids:         make(map[float64]float64),
			incomingasks: make(chan *BidAsk, 100),
			incomingbids: make(chan *BidAsk, 100),
		}
	}
	go sob.Process()
	return sob
}

func (p *SharedOrderbook) GetOrderbook(symbol string) *Orderbook {
	return p.books[symbol]
}

func (p *Orderbook) GetLowestAsk() float64 {
	p.proclock.RLock()
	defer p.proclock.RUnlock()
	lowest := float64(0)
	for price, _ := range p.asks {
		if lowest == 0 || price < lowest {
			lowest = price
		}
	}
	return lowest
}

func (p *Orderbook) GetHighestBid() float64 {
	p.proclock.RLock()
	defer p.proclock.RUnlock()
	highest := float64(0)
	for price, _ := range p.bids {
		if highest == 0 || price > highest {
			highest = price
		}
	}
	return highest
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
				if found {
					delete(p.bids, bid.price)
				}
			} else {
				p.bids[bid.price] = bid.qty
			}
		case ask := <-p.incomingasks:
			/// go through asks, clear out any that are 0 qty, update or add others
			_, found := p.asks[ask.price]
			if ask.qty == 0 {
				if found {
					delete(p.asks, ask.price)
				}
			} else {
				p.asks[ask.price] = ask.qty
			}
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
