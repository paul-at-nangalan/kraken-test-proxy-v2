package intercept

import (
	"encoding/json"
	"fmt"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	"strings"
	"time"
)

const (
	TIMEFORMAT = "2006-01-02T15:04:05.000000Z"
)

type TradeInterceptCfg struct {
	Enabled  bool
	FeeRatio float64
}

func (p *TradeInterceptCfg) Expand() {
}

type TradeIntercept struct {
	enabled  bool
	feeratio float64

	///Only the southbound thread should touch this map
	pendingtrades map[int64]Execution

	orderrequests chan *OrderReq
	traderesp     chan *Execution
	execid        int64
}

func NewTradeIntercept() *TradeIntercept {
	tradeinterceptcfg := TradeInterceptCfg{}
	err := cfg.Read("trade-intercept", &tradeinterceptcfg)
	handlers.PanicOnError(err)
	return &TradeIntercept{
		enabled:       tradeinterceptcfg.Enabled,
		feeratio:      tradeinterceptcfg.FeeRatio,
		pendingtrades: make(map[int64]Execution),
		orderrequests: make(chan *OrderReq),
		traderesp:     make(chan *Execution),
	}
}

func (p *TradeIntercept) Northbound(msg []byte) {
	if p.enabled {
		///peak at the msg
		datamap := make(map[string]interface{})
		err := json.Unmarshal(msg, &datamap)
		handlers.PanicOnError(err)
		method, ok := datamap["method"].(string)
		if ok && method == "add_order" {
			params := datamap["params"].(map[string]interface{})
			orderparams := OrderParams{
				LimitPrice:   params["limit_price"].(float64),
				OrderType:    params["order_type"].(string),
				OrderUserref: params["order_userref"].(int64),
				OrderQty:     params["order_qty"].(float64),
				Side:         params["side"].(string),
				Symbol:       params["symbol"].(string),
				Validate:     params["validate"].(bool),
				Margin:       params["margin"].(bool),
			}

			req := &OrderReq{
				Method: method,
				Params: orderparams,
				ReqId:  datamap["req_id"].(int64),
			}

			p.orderrequests <- req
		}
	}
}

func (p *TradeIntercept) Southbound(msg []byte) {
	if p.enabled {
		for len(p.orderrequests) > 0 {
			orderreq := <-p.orderrequests
			execid := fmt.Sprint("XXX", p.execid)
			p.execid++
			orderid := fmt.Sprint("XXX", orderreq.ReqId)

			fees := Fee{
				Asset: strings.Split(orderreq.Params.Symbol, "/")[0],
				Qty:   orderreq.Params.OrderQty * orderreq.Params.LimitPrice * p.feeratio,
			}
			exec := Execution{
				Cost: (orderreq.Params.OrderQty * orderreq.Params.LimitPrice) +
					(orderreq.Params.OrderQty * orderreq.Params.LimitPrice * p.feeratio),
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
			p.pendingtrades[orderreq.ReqId] = exec
		}

		///now look at the southbound message to see if it is an order resposne for any order requests

	}
}

func (p *TradeIntercept) InjectSouth() (msg []byte) {
	//TODO implement me
	panic("implement me")
}
