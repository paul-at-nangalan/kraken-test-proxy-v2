package intercept

import "time"

type OrderParams struct {
	LimitPrice   float64 `json:"limit_price"`
	OrderType    string  `json:"order_type"`
	OrderUserref int64   `json:"order_userref"`
	OrderQty     float64 `json:"order_qty"`
	Side         string  `json:"side"`
	Symbol       string  `json:"symbol"`
	Token        string  `json:"token"`
	Validate     bool    `json:"validate"`
	Margin       bool    `json:"margin"`
}
type OrderReq struct {
	Method string      `json:"method"`
	Params OrderParams `json:"params"`
	ReqId  int64       `json:"req_id"`
}

type Fee struct {
	Asset string  `json:"asset"`
	Qty   float64 `json:"qty"`
}

type Execution struct {
	Cost         float64 `json:"cost"`
	ExecId       string  `json:"exec_id"`
	ExecType     string  `json:"exec_type"`
	Fees         []Fee   `json:"fees"`
	LiquidityInd string  `json:"liquidity_ind"`
	OrdType      string  `json:"ord_type"`
	OrderId      string  `json:"order_id"`
	LastQty      float64 `json:"last_qty"`
	OrderUserref int64   `json:"order_userref"`
	LastPrice    float64 `json:"last_price"`
	Side         string  `json:"side"`
	Symbol       string  `json:"symbol"`
	Timestamp    string  `json:"timestamp"`
	TradeId      int64   `json:"trade_id"`
}

func (p *Execution) Type() string {
	return "executions"
}

func (p *Execution) Id() string {
	return "executions"
}

func (p *Execution) Data() interface{} {
	return p
}

type ExecMsg struct {
	Channel  string       `json:"channel"`
	Data     []*Execution `json:"data"`
	Sequence int64        `json:"sequence"`
	Type     string       `json:"type"`
}

type CancelParams struct {
	Orderuserref []int64 `json:"order_userref"`
}

type CancelRequest struct {
	Method string       `json:"method"`
	Params CancelParams `json:"params"`
	ReqId  int64        `json:"req_id"`
}

type CancelResult struct {
	Orderuserref int64 `json:"order_userref"`
}

type CancelResp struct {
	Method  string       `json:"method"`
	ReqId   int64        `json:"req_id"`
	Result  CancelResult `json:"result"`
	Success bool         `json:"success"`
	TimeIn  string       `json:"time_in"`
	TimeOut string       `json:"time_out"`
}

type OrderResult struct {
	OrderId      string `json:"order_id"`
	OrderUserref int64  `json:"order_userref"`
}

type OrderResp struct {
	Method  string      `json:"method"`
	ReqId   int64       `json:"req_id"`
	Result  OrderResult `json:"result"`
	Success bool        `json:"success"`
	TimeIn  time.Time   `json:"time_in"`
	TimeOut time.Time   `json:"time_out"`
}
