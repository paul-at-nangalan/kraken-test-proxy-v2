package main

import (
	"flag"
	"github.com/paul-at-nangalan/json-config/cfg"
	"kraken-test-proxy-v2/server"
)

func main() {

	cfgdir := ""
	flag.StringVar(&cfgdir, "cfg", "", "Config dir")
	flag.Parse()

	cfg.Setup(cfgdir)

	server.Listen()
}
