package intercept

import (
	"encoding/json"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	orderbooks2 "kraken-test-proxy-v2/orderbooks"
	"kraken-test-proxy-v2/recorder"
	"strings"
	"time"
)

const (
	TIMEFORMAT = "2006-01-02T15:04:05.000000Z"
)

type Filter struct {
	Matchon   string
	FilterOut bool
}

type TradeInterceptCfg struct {
	Enabled        bool
	FeeRatio       float64
	LogFilters     []Filter
	MatchOrderBook bool ///  if true, only send a trade response if the price would match an order in the order book
}

func (p *TradeInterceptCfg) Expand() {
}

type TradeIntercept struct {
	enabled  bool
	feeratio float64

	///Only the southbound thread should touch this map
	pendingtrades map[int64]*Execution
	pastrtrades   map[int64]*Execution

	orderrequests chan *OrderReq
	cancelorders  chan *CancelRequest
	/// Once we see an order response - enqueue an exec response for the next round
	traderesp  chan []*Execution
	cancelresp chan *CancelResp
	execid     int64
	sequence   int64

	enablelogging bool

	msgreplay *recorder.MessageReplay

	logfilterin  []string
	logfilterout []string

	matchorderbook bool

	orderbooks *orderbooks2.SharedOrderbook
}

func NewTradeIntercept(enablelogging bool, msgreplay *recorder.MessageReplay, orderbook *orderbooks2.SharedOrderbook) *TradeIntercept {
	tradeinterceptcfg := TradeInterceptCfg{}
	err := cfg.Read("trade-intercept", &tradeinterceptcfg)
	handlers.PanicOnError(err)

	southboundfilterin := make([]string, 0)
	southboundfilterout := make([]string, 0)
	for _, filter := range tradeinterceptcfg.LogFilters {
		if filter.FilterOut {
			southboundfilterout = append(southboundfilterout, filter.Matchon)
		} else {
			southboundfilterin = append(southboundfilterin, filter.Matchon)
		}
	}

	tradeintercept := &TradeIntercept{
		enabled:       tradeinterceptcfg.Enabled,
		feeratio:      tradeinterceptcfg.FeeRatio,
		pendingtrades: make(map[int64]*Execution),
		orderrequests: make(chan *OrderReq, 100),
		traderesp:     make(chan []*Execution, 100),
		cancelorders:  make(chan *CancelRequest, 100),
		cancelresp:    make(chan *CancelResp, 100),
		pastrtrades:   make(map[int64]*Execution),

		enablelogging: enablelogging,
		msgreplay:     msgreplay,
		logfilterin:   southboundfilterin,
		logfilterout:  southboundfilterout,

		matchorderbook: tradeinterceptcfg.MatchOrderBook,
		orderbooks:     orderbook,
	}

	return tradeintercept
}

func (p *TradeIntercept) log(msgs ...interface{}) {
	if p.enablelogging {
		fmt.Println(msgs...)
	}
}

func (p *TradeIntercept) Northbound(msg []byte) (forward bool) {
	if p.enabled {
		///peak at the msg
		datamap := make(map[string]interface{})
		err := json.Unmarshal(msg, &datamap)
		handlers.PanicOnError(err)
		method, ok := datamap["method"].(string)
		if ok {
			switch method {
			case "add_order":
				params := datamap["params"].(map[string]interface{})
				orderparams := OrderParams{
					LimitPrice:   params["limit_price"].(float64),
					OrderType:    params["order_type"].(string),
					OrderUserref: int64(params["order_userref"].(float64)),
					OrderQty:     params["order_qty"].(float64),
					Side:         params["side"].(string),
					Symbol:       params["symbol"].(string),
					Validate:     params["validate"].(bool),
					Margin:       params["margin"].(bool),
				}

				req := &OrderReq{
					Method: method,
					Params: orderparams,
					ReqId:  int64(datamap["req_id"].(float64)),
				}
				//fmt.Println("adding order request to queue")
				p.orderrequests <- req

			case "cancel_order":
				//// inject a cancel_order +ve response
				params := datamap["params"].(map[string]interface{})
				orders := params["order_userref"].([]interface{})
				cancelparams := CancelParams{
					Orderuserref: make([]int64, 0),
				}
				for _, order := range orders {
					cancelparams.Orderuserref = append(cancelparams.Orderuserref, int64(order.(float64)))
				}
				cancelorder := CancelRequest{
					Method: "cancel_order",
					Params: cancelparams,
					ReqId:  int64(datamap["req_id"].(float64)),
				}
				p.cancelorders <- &cancelorder
			case "subscribe":
				///see if this is a subscribe to the executions channel
				params := datamap["params"].(map[string]interface{})
				if params["channel"].(string) == "executions" {
					execs := p.msgreplay.Replay("executions")
					if len(execs) > 0 {
						marshalled := make([]*Execution, 0)
						for _, exec := range execs {
							marshalled = append(marshalled, exec.(*Execution))
						}
						p.traderesp <- marshalled
					}

				}
			}
		}
	}
	return true
}

func (p *TradeIntercept) handleOrderReq() {
	for len(p.orderrequests) > 0 {
		orderreq := <-p.orderrequests
		fmt.Println("pull order req from the queue")
		execid := fmt.Sprint("XXX", p.execid)
		p.execid++
		orderid := fmt.Sprint("XXX", orderreq.ReqId)

		///The cost ought to be in the currency being sold
		cost := float64(0)
		if orderreq.Params.Side == "buy" {
			///if we are buying, then the cost is the qty of the 1st currency
			cost = (orderreq.Params.OrderQty) +
				(orderreq.Params.OrderQty * p.feeratio)
		} else {
			///if we are selling, the cost is the qty of the 2nd currency
			cost = (orderreq.Params.OrderQty * orderreq.Params.LimitPrice) +
				(orderreq.Params.OrderQty * orderreq.Params.LimitPrice * p.feeratio)
		}

		fees := Fee{
			Asset: strings.Split(orderreq.Params.Symbol, "/")[1],
			/// qty of 1st * price of 2nd = qty of 2nd. qty of 2nd * fees ratio = total fees
			Qty: orderreq.Params.OrderQty * orderreq.Params.LimitPrice * p.feeratio,
		}
		exec := Execution{
			//// This is not clearly defined on Kraken docs ... but I think it should be how much we had to sell (whether its a buy or sell order)
			///   of an asset to get the other asset
			Cost:         cost,
			ExecId:       execid,
			ExecType:     "trade",
			Fees:         []Fee{fees},
			LiquidityInd: "m",
			OrdType:      "limit",
			OrderId:      orderid,
			LastQty:      orderreq.Params.OrderQty,
			OrderUserref: orderreq.Params.OrderUserref,
			LastPrice:    orderreq.Params.LimitPrice,
			Side:         orderreq.Params.Side,
			Symbol:       orderreq.Params.Symbol,
			Timestamp:    time.Now().Format(TIMEFORMAT),
			TradeId:      orderreq.Params.OrderUserref,
		}
		p.log("move exec to the map by order user ref ", exec.OrderUserref, " ", exec.ExecId)
		p.pendingtrades[orderreq.Params.OrderUserref] = &exec
	}

}

func (p *TradeIntercept) CheckFilters(msg []byte) bool {
	for _, filter := range p.logfilterin {
		if strings.Contains(string(msg), filter) {
			return true
		}
	}
	for _, filter := range p.logfilterout {
		if strings.Contains(string(msg), filter) {
			return false
		}

	}
	return true
}

func (p *TradeIntercept) findAndQueueMatchedTrades() {
	if p.matchorderbook {
		///find and queue any matched trades
		for _, exec := range p.pendingtrades {
			//// See if the order would be filled based on latest orderbook data
			orderbook := p.orderbooks.GetOrderbook(exec.Symbol)
			isfilled := false
			fillprice := 0.0
			fillqty := 0.0
			if exec.Side == "buy" {
				fillprice, fillqty = orderbook.MatchBid(exec.LastPrice, exec.LastQty)
				if fillqty > 0 {
					isfilled = true
				}
			} else {
				fillprice, fillqty = orderbook.MatchAsk(exec.LastPrice, exec.LastQty)
				if fillqty > 0 {
					isfilled = true
				}
			}

			if isfilled {
				exec.LastPrice = fillprice
				exec.LastQty = fillqty
				p.traderesp <- []*Execution{exec}
				p.msgreplay.AddMessage(exec)
				delete(p.pendingtrades, exec.OrderUserref)
			}
		}
	}
}

// // See if this order is in the pending trades map - if it is, clear it and put it on the past trades map
func (p *TradeIntercept) cancelPendingTrades(cancelreq *CancelRequest) {
	for _, order := range cancelreq.Params.Orderuserref {
		exec, ok := p.pendingtrades[order]
		if ok {
			p.log("canceling trade ", exec.OrderUserref, " ", exec.ExecId)
			p.pastrtrades[exec.OrderUserref] = exec
			delete(p.pendingtrades, order)
		}
	}
}

func (p *TradeIntercept) processCancelOrder(datamap map[string]interface{}, msg []byte) bool {
	success := datamap["success"].(bool)

	if !success {
		if len(p.cancelorders) > 0 {
			//replace this message with a success message for all cancellations
			p.log("Replacing ", string(msg), " with successful cancel")
			orders := <-p.cancelorders
			p.cancelPendingTrades(orders)
			for _, order := range orders.Params.Orderuserref {

				cancelresp := &CancelResp{
					Method: "cancel_order",
					ReqId:  int64(datamap["req_id"].(float64)),
					Result: CancelResult{
						Orderuserref: order,
					},
					Success: true,
					TimeIn:  time.Now().Format(TIMEFORMAT),
					TimeOut: time.Now().Format(TIMEFORMAT),
				}
				p.cancelresp <- cancelresp
			}
		}

		return false
	} else {
		if len(p.cancelorders) > 0 {
			///deque the request
			orders := <-p.cancelorders
			p.cancelPendingTrades(orders)
		}
	}
	return true
}

func (p *TradeIntercept) processAddOrder(datamap map[string]interface{}, msg []byte) bool {
	result := datamap["result"].(map[string]interface{})
	orderresult := OrderResult{
		OrderId:      "",
		OrderUserref: int64(result["order_userref"].(float64)),
	}
	orderressp := OrderResp{
		Method:  "",
		ReqId:   int64(datamap["req_id"].(float64)),
		Result:  orderresult,
		Success: datamap["success"].(bool),
		TimeIn:  time.Time{}, ///don't care
		TimeOut: time.Time{},
	}
	/// Full test mode - simply send a trade in response as soon as we see the order response
	/// Otherwise we must look for an orderbook match
	if !p.matchorderbook {
		exectrade, ok := p.pendingtrades[orderressp.Result.OrderUserref]
		if ok {
			fmt.Println("push exec to queue")
			p.traderesp <- []*Execution{exectrade}
			p.msgreplay.AddMessage(exectrade)
			delete(p.pendingtrades, orderressp.Result.OrderUserref)
		}
	}
	return true
}

func (p *TradeIntercept) processBook(datamap map[string]interface{}) {
	///firstly get the symbol from the datamap, and use that to get the orderbook from the shared orderbook
	symbol := datamap["symbol"].(string)
	orderbook := p.orderbooks.GetOrderbook(symbol)
	/// The datamap should have a key "data" containing a map of "asks" and "bids"
	///   each of which is an array of maps with keys "price" and "qty"
	bids := datamap["data"].(map[string]interface{})["bids"].([]interface{})
	asks := datamap["data"].(map[string]interface{})["asks"].([]interface{})
	marshalledbids := make([]*orderbooks2.BidAsk, len(bids))
	for _, bid := range bids {
		bidmap := bid.(map[string]interface{})
		marshalledbids = append(marshalledbids, orderbooks2.NewBidAsk(bidmap["price"].(float64), bidmap["qty"].(float64)))
	}
	orderbook.AddBids(marshalledbids)
	/// now do the same for asks
	marshalledasks := make([]*orderbooks2.BidAsk, len(asks))
	for _, ask := range asks {
		askmap := ask.(map[string]interface{})
		marshalledasks = append(marshalledasks, orderbooks2.NewBidAsk(askmap["price"].(float64), askmap["qty"].(float64)))
	}
	orderbook.AddAsks(marshalledasks)
}

func (p *TradeIntercept) Southbound(msg []byte) (forward bool) {
	if p.enabled {
		//// dequeu any previous northbound order requests and put into a map - this is to avoid 2 threads accessing the map
		///   this puts a execution type onto the map - all we need to do then is wait for the corresponding
		///   south bound add_order success message and inject the execution _after_ by putting it on the traderesp queue
		p.handleOrderReq()
		p.findAndQueueMatchedTrades()

		///now look at the southbound message to see if it is an order resposne for any order requests
		datamap := make(map[string]interface{})
		err := json.Unmarshal(msg, &datamap)
		handlers.PanicOnError(err)
		method, ok := datamap["method"].(string)
		if ok && method == "add_order" {
			if !p.processAddOrder(datamap, msg) {
				return false
			}
		}
		if ok && method == "cancel_order" {
			if !p.processCancelOrder(datamap, msg) {
				return false
			}
		}
		channel, ok := datamap["channel"].(string)
		if ok && channel == "book" {
			if p.matchorderbook {
				p.processBook(datamap)
			}
		}
	}
	return true
}

func (p *TradeIntercept) InjectSouth() (msg []byte) {
	if p.enabled {

		if len(p.traderesp) > 0 {
			exec := <-p.traderesp
			execmsg := &ExecMsg{
				Channel:  "executions",
				Data:     exec,
				Sequence: p.sequence,
				Type:     "snapshot",
			}
			msg, err := json.Marshal(execmsg)
			handlers.PanicOnError(err)
			//fmt.Println("sending exec response ", string(msg)) ///DEBUG
			return msg
		} else if len(p.cancelresp) > 0 {
			cancelresp := <-p.cancelresp
			msg, err := json.Marshal(cancelresp)
			handlers.PanicOnError(err)
			//p.log("sending cancel response ", string(msg))
			return msg
		}
	}
	return nil
}
