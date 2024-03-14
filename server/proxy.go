package server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	"kraken-test-proxy-v2/client"
	"kraken-test-proxy-v2/intercept"
	orderbooks2 "kraken-test-proxy-v2/orderbooks"
	"kraken-test-proxy-v2/recorder"
	"log"
	"net/http"
	"os"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Config struct {
	Certfile string
	Keyfile  string
	Port     string

	LogPrivate bool
	LogPublic  bool

	OrderbookSymbols []string
}

func (p *Config) Expand() {
	p.Certfile = os.ExpandEnv(p.Certfile)
	p.Keyfile = os.ExpandEnv(p.Keyfile)
}

var cfgsvr *Config
var orderbooks *orderbooks2.SharedOrderbook

type Intercept interface {
	Northbound(msg []byte) (forward bool)
	Southbound(msg []byte) (forward bool)

	InjectSouth() (msg []byte)             /// nil for no message
	CheckFilters(msg []byte) (logmsg bool) /// For whether to log the message or not
}

type WebSockProxy struct {
	intercept     Intercept
	conn          *websocket.Conn
	relay         *client.Relay
	enablelogging bool
}

func NewWebSockProxy(intercept Intercept, conn *websocket.Conn, relay *client.Relay, enablelogging bool) *WebSockProxy {
	wsp := &WebSockProxy{
		intercept:     intercept,
		conn:          conn,
		relay:         relay,
		enablelogging: enablelogging,
	}
	return wsp
}

var msgreplay *recorder.MessageReplay

func Listen() {
	cfgsvr = &Config{}
	err := cfg.Read("server", cfgsvr)
	handlers.PanicOnError(err)

	orderbooks = orderbooks2.NewSharedOrderbook(cfgsvr.OrderbookSymbols)

	http.HandleFunc("/private", wsHandlerPrivate)
	http.HandleFunc("/public", wsHandlerPublic)

	////Create a message replayer
	msgreplay = recorder.NewMessageReplay()

	err = http.ListenAndServeTLS(cfgsvr.Port, cfgsvr.Certfile, cfgsvr.Keyfile, nil)
	handlers.PanicOnError(err)
}

func (p *WebSockProxy) logmsg(msg []byte, preffix string) {
	if p.enablelogging && p.intercept.CheckFilters(msg) {
		fmt.Println(preffix, " - ", time.Now().Format("2006-01-02 15:04:05"), ":", string(msg))
	}
}

func (p *WebSockProxy) southbound() {
	defer handlers.HandlePanic()
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		injectmsg := p.intercept.InjectSouth()
		if injectmsg != nil {
			p.logmsg(injectmsg, "s-inj")
			err := p.conn.WriteMessage(websocket.BinaryMessage, injectmsg)
			if err != nil {
				log.Println("Send error south", err)
				return
			}
		}

		msg, err := p.relay.RecvMsg()
		if err != nil {
			log.Println("Recv error north", err)
			return
		}
		p.logmsg(msg, "s")
		if !p.intercept.Southbound(msg) {
			p.logmsg(msg, "s-dropped")
			continue
		}

		err = p.conn.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			log.Println("Send error south", err)
			return
		}
	}
}

func (p *WebSockProxy) northbound() {
	defer handlers.HandlePanic()
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		_, message, err := p.conn.ReadMessage()
		if err != nil {
			log.Println("Recv error south", err)
			return
		}
		p.logmsg(message, "n")
		if !p.intercept.Northbound(message) {
			p.logmsg(message, "n-dropped")
			continue
		}
		err = p.relay.SendMsg(message)
		if err != nil {
			log.Println("Send error north", err)
			return
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request, private bool) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println("Connecting private channel", private)
	relay := client.Connect(private)

	enablelogging := false
	if private && cfgsvr.LogPrivate {
		enablelogging = true
	} else if !private && cfgsvr.LogPublic {
		enablelogging = true
	}

	msgintercept := intercept.NewTradeIntercept(enablelogging, msgreplay)
	wshandler := NewWebSockProxy(msgintercept, conn, relay, enablelogging)

	go wshandler.southbound()
	go wshandler.northbound()
}
func wsHandlerPrivate(w http.ResponseWriter, r *http.Request) {
	wsHandler(w, r, true)
}
func wsHandlerPublic(w http.ResponseWriter, r *http.Request) {
	wsHandler(w, r, false)
}
