package client

import (
	"github.com/gorilla/websocket"
	"github.com/paul-at-nangalan/errorhandler/handlers"
	"github.com/paul-at-nangalan/json-config/cfg"
	"os"
	"time"
)

type Relay struct {
	conn    *websocket.Conn
	timeout time.Duration
}

type KrakenCfg struct {
	UrlPrivate string `json:"UrlPrivate"`
	Apikey     string `json:"Apikey"`
	Apisecret  string `json:"Apisecret"`
	Timeout    string
}

func (p *KrakenCfg) Expand() {
	p.UrlPrivate = os.ExpandEnv(p.UrlPrivate)
	p.Apikey = os.ExpandEnv(p.Apikey)
	p.Apisecret = os.ExpandEnv(p.Apisecret)
}

func Connect() *Relay {
	krkcfg := KrakenCfg{}
	err := cfg.Read("kraken", &krkcfg)
	handlers.PanicOnError(err)

	// Connect to Kraken WebSocket API
	conn, _, err := websocket.DefaultDialer.Dial("wss://"+krkcfg.UrlPrivate, nil)
	handlers.PanicOnError(err)
	timeout, err := time.ParseDuration(krkcfg.Timeout)
	handlers.PanicOnError(err)
	return &Relay{
		conn:    conn,
		timeout: timeout,
	}
}

func (p *Relay) Close() {
	p.conn.Close()
}

func (p *Relay) SendMsg(data []byte) error {
	p.conn.SetWriteDeadline(time.Now().Add(p.timeout))
	err := p.conn.WriteMessage(websocket.BinaryMessage, data)
	return err
}

func (p *Relay) RecvMsg() (data []byte, err error) {
	p.conn.SetReadDeadline(time.Now().Add(p.timeout))
	_, data, err = p.conn.ReadMessage()
	return data, err
}
