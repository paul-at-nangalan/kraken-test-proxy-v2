# kraken-test-proxy-v2
A very basic test proxy for Kraken Websock v2 which injects trades when test orders are placed.

## Build

You must have Go (Golang) installed

#### Get the source code
git clone https://github.com/paul-at-nangalan/kraken-test-proxy-v2.git

#### In the project root directory kraken-test-proxy-v2
go install

## Config
### Proxy
You probably don't need to change the config for the proxy except for fees, which are in cfg/trade-intercept.json

### Your trading engine
Public URL of the trading engine: 127.0.0.1:8443/public
Private URL of the trading engine: 127.0.0.1:8443/private

## Install
If the PATH env variable is pointed at the go/bin directory, there's no need to install, but you might want to
copy the cfg/ directory to a seperate location

## Running
kraken-test-proxy-v2 --cfg ./cfg

