package orderbooks

import "testing"
import "github.com/stretchr/testify/assert"

func TestOrderBookMatch(t *testing.T) {
	orderbook := NewOrderBook()
	///Create some random bids and asks
	bids := []*BidAsk{
		{price: 37500, qty: 1.25},
		{price: 37400, qty: 1.25},
		{price: 37300, qty: 1.25},
		{price: 37200, qty: 1.25},
		{price: 37100, qty: 1.25},
	}
	asks := []*BidAsk{
		{price: 37600, qty: 1.25},
		{price: 37700, qty: 1.25},
		{price: 37800, qty: 1.25},
		{price: 37900, qty: 1.25},
		{price: 38000, qty: 1.25},
	}
	///Add the bids and asks to the orderbook
	orderbook.AddBids(bids)
	orderbook.process()
	orderbook.AddAsks(asks)
	orderbook.process()

	///Now create a bid and ask that should match
	price := 37401.0
	qty := 1.25
	fillprice, fillqty := orderbook.MatchAsk(price, qty)
	assert.Equal(t, 37401.0, fillprice)
	assert.Equal(t, 1.25, fillqty)
	//Now create a bid and ask that should not match
	price = 37501.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchAsk(price, qty)
	assert.Equal(t, 0.0, fillprice)
	assert.Equal(t, 0.0, fillqty)
	///Now create an ask that should match
	/// bid for a price higher than the second highest ask - we should fill at our price
	price = 37701.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchBid(price, qty)
	assert.Equal(t, 37701.0, fillprice)
	assert.Equal(t, 1.25, fillqty)
	///Now create an bid higher than the highest ask - it should not fill
	price = 37599.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchBid(price, qty)
	assert.Equal(t, 0.0, fillprice)
	assert.Equal(t, 0.0, fillqty)

}

func TestDirtyFlag(t *testing.T) {
	orderbook := NewOrderBook()
	///Create some random bids and asks
	bids := []*BidAsk{
		{price: 37500, qty: 1.25},
		{price: 37400, qty: 1.25},
		{price: 37300, qty: 1.25},
		{price: 37200, qty: 1.25},
		{price: 37100, qty: 1.25},
	}
	asks := []*BidAsk{
		{price: 37600, qty: 1.25},
		{price: 37700, qty: 1.25},
		{price: 37800, qty: 1.25},
		{price: 37900, qty: 1.25},
		{price: 38000, qty: 1.25},
	}
	///Add the bids and asks to the orderbook
	orderbook.AddBids(bids)
	orderbook.process()

	orderbook.AddAsks(asks)
	orderbook.process()

	///Now create an ask higher than the highest bid - it should not fill
	price := 37601.0
	qty := 1.25
	fillprice, fillqty := orderbook.MatchAsk(price, qty)
	assert.Equal(t, 0.0, fillprice)
	assert.Equal(t, 0.0, fillqty)

	///Now add a bid lower than the lowest ask - it should not fill
	price = 37099.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchBid(price, qty)
	assert.Equal(t, 0.0, fillprice)
	assert.Equal(t, 0.0, fillqty)
	///Now add some more bids higher than our ask and asks lower than our bid - they should now fill
	bids = []*BidAsk{
		{price: 37700, qty: 1.25}, /// this should fill the ask at 37601
		{price: 37601, qty: 1.25}, /// this should fill the ask at 37601
		{price: 37300, qty: 1.25},
		{price: 37200, qty: 1.25},
		{price: 37100, qty: 1.25},
		{price: 37000, qty: 1.25},
	}
	asks = []*BidAsk{
		{price: 37000, qty: 1.25}, //this should match our bid at 37099
		{price: 37700, qty: 1.25},
		{price: 37800, qty: 1.25},
		{price: 37900, qty: 1.25},
		{price: 38000, qty: 1.25},
		{price: 38100, qty: 1.25},
	}
	///add the bids and asks to the orderbook
	orderbook.AddBids(bids)
	orderbook.process()

	orderbook.AddAsks(asks)
	orderbook.process()

	///Create a bid that is lower than the lowest ask and an ask that is higher than the highest bid
	/// to test that we annot place the order
	price = 37099.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchBid(price, qty)
	assert.Equal(t, 37099.0, fillprice)
	assert.Equal(t, 1.25, fillqty)
	price = 37601.0
	qty = 1.25
	fillprice, fillqty = orderbook.MatchAsk(price, qty)
	assert.Equal(t, 37601.0, fillprice)
	assert.Equal(t, 1.25, fillqty)

	///and we're done
}
