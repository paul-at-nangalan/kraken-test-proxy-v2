package server

import (
	"github.com/gorilla/websocket"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	"kraken-test-proxy-v2/client"
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
	cfg.Read("server", &cfgsvr)

	http.HandleFunc("/", wsHandler)
	err := http.ListenAndServeTLS(":443", cfgsvr.Certfile, cfgsvr.Keyfile, nil)
	handlers.PanicOnError(err)
}

func (p *WebSockProxy) southbound() {
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		injectmsg := p.intercept.InjectSouth()
		if injectmsg != nil {
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
		p.intercept.Southbound(msg)

		err = p.conn.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			log.Println("Send error south", err)
			return
		}
	}
}

func (p *WebSockProxy) northbound() {
	defer p.conn.Close()
	defer p.relay.Close()
	for {
		_, message, err := p.conn.ReadMessage()
		if err != nil {
			log.Println("Recv error south", err)
			return
		}
		p.intercept.Northbound(message)
		err = p.relay.SendMsg(message)
		if err != nil {
			log.Println("Send error north", err)
			return
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	relay := client.Connect()

	wshandler := NewWebSockProxy(nil, conn, relay)

	go wshandler.southbound()
	go wshandler.northbound()
}
