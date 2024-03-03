package server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	"kraken-test-proxy-v2/client"
	"kraken-test-proxy-v2/intercept"
	"log"
	"net/http"
	"os"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Config struct {
	Certfile string
	Keyfile  string
	Port     string
}

func (p *Config) Expand() {
	p.Certfile = os.ExpandEnv(p.Certfile)
	p.Keyfile = os.ExpandEnv(p.Keyfile)
}

type Intercept interface {
	Northbound(msg []byte)
	Southbound(msg []byte)

	InjectSouth() (msg []byte) /// nil for no message
}

type WebSockProxy struct {
	intercept Intercept
	conn      *websocket.Conn
	relay     *client.Relay
}

func NewWebSockProxy(intercept Intercept, conn *websocket.Conn, relay *client.Relay) *WebSockProxy {
	wsp := &WebSockProxy{
		intercept: intercept,
		conn:      conn,
		relay:     relay,
	}
	return wsp
}

func Listen() {
	cfgsvr := Config{}
	err := cfg.Read("server", &cfgsvr)
	handlers.PanicOnError(err)

	http.HandleFunc("/private", wsHandlerPrivate)
	http.HandleFunc("/public", wsHandlerPublic)
	err = http.ListenAndServeTLS(cfgsvr.Port, cfgsvr.Certfile, cfgsvr.Keyfile, nil)
	handlers.PanicOnError(err)
}

func (p *WebSockProxy) southbound() {
	defer handlers.HandlePanic()
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		fmt.Println("s1")
		injectmsg := p.intercept.InjectSouth()
		if injectmsg != nil {
			fmt.Println("s2")
			err := p.conn.WriteMessage(websocket.BinaryMessage, injectmsg)
			if err != nil {
				log.Println("Send error south", err)
				return
			}
			fmt.Println("s2.1")
		}
		fmt.Println("s3")

		msg, err := p.relay.RecvMsg()
		if err != nil {
			log.Println("Recv error north", err)
			return
		}
		fmt.Println("s4")
		p.intercept.Southbound(msg)
		fmt.Println("s5")

		err = p.conn.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			log.Println("Send error south", err)
			return
		}
		fmt.Println("s6")
	}
}

func (p *WebSockProxy) northbound() {
	defer handlers.HandlePanic()
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		fmt.Println("n1")
		_, message, err := p.conn.ReadMessage()
		if err != nil {
			log.Println("Recv error south", err)
			return
		}
		fmt.Println("n2")
		p.intercept.Northbound(message)
		fmt.Println("n3")
		err = p.relay.SendMsg(message)
		if err != nil {
			log.Println("Send error north", err)
			return
		}
		fmt.Println("n4")
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

	msgintercept := intercept.NewTradeIntercept()
	wshandler := NewWebSockProxy(msgintercept, conn, relay)

	go wshandler.southbound()
	go wshandler.northbound()
}
func wsHandlerPrivate(w http.ResponseWriter, r *http.Request) {
	wsHandler(w, r, true)
}
func wsHandlerPublic(w http.ResponseWriter, r *http.Request) {
	wsHandler(w, r, false)
}
