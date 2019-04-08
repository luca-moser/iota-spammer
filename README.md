# Spammer

An IOTA transaction spammer.

Modes:
* Spam using `getTransactionsToApprove`.
* Spam using tips from a pool of txs filled by a ZMQ stream of txs.

## Installation

1. Install [Go](https://golang.org/dl/)
2. `sudo apt install build-essential`
3. Install ZMQ according to [this](https://gist.github.com/katopz/8b766a5cb0ca96c816658e9407e83d00)
4. Install ZMQ-dev via `sudo apt install libzmq3-dev` 
5. Clone this repository into a folder outside of `$GOPATH`

Make sure to read the default CLI flag values below as the default values are specifically trimmed for
a private net. In order to use the spammer on mainnet, the CLI flags have to be changed.

## Run

### Standard
Execute the spammer for example with `go run -tags="pow_avx" main.go`, this will let
the spammer use `getTransactionsToApprove` with the default depth of `1` using the node located
at `http://127.0.0.1:14265`.

You can switch the `-tags="pow_avx"` part to the supported PoW implementations of the
[iota.go](https://github.com/iotaledger/iota.go) library, for example `pow_sse`, `pow_c` etc.

### Maximizing Throughput
The spammer can use a pool of txs gotten from a ZMQ stream in order to maximize throughput
(instead of using `geTransactionsToApprove` which is much slower).

For example `go run -tags="pow_avx" main.go -zmq` will instruct the spammer to first try to fill
up a pool of txs via spam txs using `getTransactionsToApprove`, until the default pool size 
is filled (`50`) and then subsequently use that pool of txs as tips for the own
issued spam txs. The pool of txs will be updated continuously from the ZMQ stream (while spamming is ongoing).

When nodes are not responsive against `getTransactionsToApprove` calls, then `-zmq-no-tip-sel`
should be used to instruct the spammer to simply wait for the pool of txs to be filled
instead of trying to fill it by initially using spam txs which use `getTransactionsToApprove`.

`-bc-batch-size` defines the amount of txs to retain before broadcasting them using the defined node.
Changing this value can change the throughput quite drastically.

## Flags
* -instances, spammer instance counts; default: 5
* -node, node to use, default: http://127.0.0.1:14265
* -depth, depth for `getTransactionsToApprove`; default: 1
* -mwm, mwm for pow; default: 1
* -tag, tag of txs, default: "SPAMMER"
* -zmq, use a zmq stream of txs as tips, default: false
* -zmq-url, the url of the zmq stream, default: tcp://127.0.0.1:5556
* -zmq-buf, the size of the zmq tx ring buffer; default: 50
* -zmq-no-tip-sel, whether to not perform normal spam with tip-selection until the zmq buffer is full, default: false
* -bc-batch-size, how many txs to batch before submitting them to the node, default: 100