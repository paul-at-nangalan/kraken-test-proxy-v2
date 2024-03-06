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
	cancelorders  chan *CancelRequest
	/// Once we see an order response - enqueue an exec response for the next round
	traderesp  chan *Execution
	cancelresp chan *CancelResp
	execid     int64
	sequence   int64

	enablelogging bool
}

func NewTradeIntercept(enablelogging bool) *TradeIntercept {
	tradeinterceptcfg := TradeInterceptCfg{}
	err := cfg.Read("trade-intercept", &tradeinterceptcfg)
	handlers.PanicOnError(err)
	return &TradeIntercept{
		enabled:       tradeinterceptcfg.Enabled,
		feeratio:      tradeinterceptcfg.FeeRatio,
		pendingtrades: make(map[int64]Execution),
		orderrequests: make(chan *OrderReq, 100),
		traderesp:     make(chan *Execution, 100),
		cancelorders:  make(chan *CancelRequest, 100),
		cancelresp:    make(chan *CancelResp, 100),

		enablelogging: enablelogging,
	}
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
				fmt.Println("adding order request to queue")
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
		fmt.Println("push exec to queue")
		p.pendingtrades[orderreq.ReqId] = exec
	}

}

func (p *TradeIntercept) Southbound(msg []byte) (forward bool) {
	if p.enabled {
		//// dequeu any previous northbound order requests and put into a map - this is to avoid 2 threads accessing the map
		///   this puts a execution type onto the map - all we need to do then is wait for the corresponding
		///   south bound add_order success message and inject the execution _after_ by putting it on the traderesp queue
		p.handleOrderReq()

		///now look at the southbound message to see if it is an order resposne for any order requests
		datamap := make(map[string]interface{})
		err := json.Unmarshal(msg, &datamap)
		handlers.PanicOnError(err)
		method, ok := datamap["method"].(string)
		if ok && method == "add_order" {
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
			exectrade, ok := p.pendingtrades[orderressp.Result.OrderUserref]
			if ok {
				p.traderesp <- &exectrade
				delete(p.pendingtrades, orderressp.Result.OrderUserref)
			}
		}
		if ok && method == "cancel_order" {
			success := datamap["success"].(bool)
			if !success {
				if len(p.cancelorders) > 0 {
					//replace this message with a success message for all cancellations
					p.log("Replacing ", string(msg), " with successful cancel")
					orders := <-p.cancelorders
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
					<-p.cancelorders
				}
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
				Data:     []*Execution{exec},
				Sequence: p.sequence,
				Type:     "snapshot",
			}
			msg, err := json.Marshal(execmsg)
			handlers.PanicOnError(err)
			//p.log("sending exec response ", string(msg))
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
