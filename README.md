# Spammer

An IOTA transaction spammer.

Modes:
* Spam using `getTransactionsToApprove`.
* Spam using tips from a buffer filled by a transaction ZMQ stream.

Flags:
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