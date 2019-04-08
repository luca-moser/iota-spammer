package main

import (
	"container/ring"
	"flag"
	"fmt"
	"github.com/iotaledger/iota.go/account"
	"github.com/iotaledger/iota.go/account/builder"
	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/bundle"
	"github.com/iotaledger/iota.go/checksum"
	"github.com/iotaledger/iota.go/consts"
	"github.com/iotaledger/iota.go/pow"
	"github.com/iotaledger/iota.go/transaction"
	"github.com/iotaledger/iota.go/trinary"
	"github.com/pebbe/zmq4"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var instancesNum = flag.Int("instances", 5, "spammer instance counts")
var node = flag.String("node", "http://127.0.0.1:14265", "node to use")
var depth = flag.Int("depth", 1, "depth for gtta")
var mwm = flag.Int("mwm", 1, "mwm for pow")
var tag = flag.String("tag", "SPAMMER", "tag of txs")
var zmq = flag.Bool("zmq", false, "use a zmq stream of txs as tips")
var zmqURL = flag.String("zmq-url", "tcp://127.0.0.1:5556", "the url of the zmq stream")
var zmqBuf = flag.Int("zmq-buf", 50, "the size of the zmq tx ring buffer")
var zmqNoTipSel = flag.Bool("zmq-no-tip-sel", false, "whether to not perform normal spam with tip-selection until the zmq buffer is full")
var bcBatchSize = flag.Int("bc-batch-size", 100, "how many txs to batch before submitting them to the node")

var targetAddr trinary.Hash
var emptySeed = strings.Repeat("9", 81)

func main() {
	flag.Parse()

	target := strings.Repeat("9", 81)
	var err error
	targetAddr, err = checksum.AddChecksum(target, true, consts.AddressChecksumTrytesSize)
	must(err)

	if *zmq {
		for i := 0; i < *instancesNum; i++ {
			go zmqSpammer()
		}
		if !*zmqNoTipSel {
			accSpammer(*zmqBuf)
		}
	} else {
		for i := 0; i < *instancesNum; i++ {
			accSpammer(-1)
		}
	}
	pad := strings.Repeat("", 10)
	const pointsCount = 5
	points := [pointsCount]int64{}
	var index int
	var tps float64
	for {
		s := atomic.LoadInt64(&spammed)
		points[index] = s
		index++
		if index == 5 {
			index = 0
			var deltaSum int64
			for i := 0; i < pointsCount-1; i++ {
				deltaSum += points[i+1] - points[i]
			}
			tps = float64(deltaSum) / float64(pointsCount)
		}
		fmt.Printf("%s\r", pad)
		fmt.Printf("\rspammed %d (tps %.2f)", s, tps)
		<-time.After(time.Duration(1) * time.Second)
	}
	<-make(chan struct{})
}

func accSpammer(stopAfter int) {
	_, powFunc := pow.GetFastestProofOfWorkImpl()
	_ = powFunc
	iotaAPI, err := api.ComposeAPI(api.HTTPClientSettings{URI: *node, LocalProofOfWorkFunc: powFunc})
	must(err)

	acc, err := builder.NewBuilder().WithAPI(iotaAPI).WithMWM(uint64(*mwm)).WithDepth(uint64(*depth)).Build()
	must(err)
	must(acc.Start())

	go func() {
		for {
			_, err := acc.Send(account.Recipient{Address: targetAddr, Tag: *tag});
			if err != nil {
				fmt.Printf("error sending: %s\n", err.Error())
				continue
			}
			if stopAfter != -1 {
				stopAfter--
				if stopAfter == 0 {
					break
				}
			} else {
				atomic.AddInt64(&spammed, 1)
			}
		}
	}()
}

var spammed int64 = 0

func zmqSpammer() {
	socket, err := zmq4.NewSocket(zmq4.SUB)
	must(err)
	must(socket.SetSubscribe("tx"))
	err = socket.Connect(*zmqURL)
	must(err)

	var rMu sync.Mutex
	r := ring.New(*zmqBuf)

	go func() {
		for {
			msg, err := socket.Recv(0)
			must(err)
			split := strings.Split(msg, " ")
			if len(split) != 13 {
				continue
			}

			rMu.Lock()
			r.Value = split[1]
			r = r.Next()
			rMu.Unlock()
		}
	}()

	// wait for ring buffer to be filled up
	for {
		ready := true
		filled := 0
		r.Do(func(v interface{}) {
			if v == nil {
				ready = false
				return
			}
			filled++
		})
		if ready {
			break
		}
		fmt.Printf("\rwaiting for ring buffer to be filled (%d/%d)", filled, *zmqBuf)
		<-time.After(time.Duration(1) * time.Second)
	}
	fmt.Printf("\nbuffer filled...")

	_, powFunc := pow.GetFastestProofOfWorkImpl()
	_ = powFunc
	iotaAPI, err := api.ComposeAPI(api.HTTPClientSettings{URI: *node, LocalProofOfWorkFunc: powFunc})
	must(err)

	spamTransfer := []bundle.Transfer{{Address: targetAddr, Tag: *tag}}
	bndl, err := iotaAPI.PrepareTransfers(emptySeed, spamTransfer, api.PrepareTransfersOptions{})
	must(err)

	for {
		toBroadcast := []trinary.Trytes{}
		for i := 0; i < *bcBatchSize; i++ {
			rMu.Lock()
			trunk := r.Prev().Value.(string)
			branch := r.Next().Value.(string)
			rMu.Unlock()

			powedBndl, err := iotaAPI.AttachToTangle(trunk, branch, uint64(*mwm), bndl)
			must(err)

			tx, err := transaction.AsTransactionObject(powedBndl[0])
			must(err)
			hash := transaction.TransactionHash(tx)
			rMu.Lock()
			r.Value = hash
			r = r.Next()
			rMu.Unlock()
			toBroadcast = append(toBroadcast, powedBndl[0])
		}

		_, err = iotaAPI.BroadcastTransactions(toBroadcast...)
		must(err)
		atomic.AddInt64(&spammed, int64(*bcBatchSize))
		toBroadcast = []trinary.Trytes{}
	}
}
