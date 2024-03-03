# kraken-test-proxy-v2
I've made this public in case someone finds it useful.
It's just a very basic test proxy for Kraken Websock v2 which injects trades when test orders are placed.

*** WARNING: this is a proxy - all orders will be forwarded to the exchange - make sure the test only (validate)
***   param is set correctly in the order (see Kraken web socket v2 docs)

## Build

You must have Go (Golang) installed

#### Get the source code
git clone https://github.com/paul-at-nangalan/kraken-test-proxy-v2.git

#### In the project root directory kraken-test-proxy-v2
go install

## Config
### Proxy
Set up the certificate config for the server, server.json:
Certfile: this should point to a concatenation of the server cert and CA certs
Keyfile: this should point to the private key file

You can enable logging with these params in the server.json:
LogPrivate
LogPublic

For _testing_ with this proxy, you will probably need to set insecure mode in the websocket dialer of your trading engine, something like this:
```
dialer.TLSClientConfig = &tls.Config{
    InsecureSkipVerify: true,
}
```

### Your trading engine
Public URL of the trading engine: 127.0.0.1:8443/public
Private URL of the trading engine: 127.0.0.1:8443/private

## Install
If the PATH env variable is pointed at the go/bin directory, there's no need to install, but you might want to
copy the cfg/ directory to a seperate location

## Running
kraken-test-proxy-v2 --cfg ./cfg

## Creating a specific interceptor

You can create your own interceptor, it simply needs to implement the Intercept interface.
```
type Intercept interface {
    Northbound(msg []byte)
    Southbound(msg []byte)

    InjectSouth() (msg []byte) /// nil for no message
}
```
And then load it into the proxy in the wsHandler function in server/proxy.go