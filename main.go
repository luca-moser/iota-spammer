package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/iotaledger/iota.go/address"
	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/bundle"
	"github.com/iotaledger/iota.go/checksum"
	"github.com/iotaledger/iota.go/consts"
	"github.com/iotaledger/iota.go/converter"
	. "github.com/iotaledger/iota.go/guards/validators"
	"github.com/iotaledger/iota.go/pow"
	"github.com/iotaledger/iota.go/signing"
	"github.com/iotaledger/iota.go/transaction"
	"github.com/iotaledger/iota.go/trinary"
	powsrvio "gitlab.com/powsrv.io/go/client"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var instancesNum = flag.Int("instances", 5, "spammer instance counts")
var node = flag.String("node", "http://127.0.0.1:14265", "node to use")
var powsrvioKey = flag.String("powsrvio-key", "", "the powsrv.io key to use")
var nodes = flag.String("nodes", "", "nodes to use")
var depth = flag.Int("depth", 1, "depth for gtta")
var mwm = flag.Int("mwm", 1, "mwm for pow")
var tag = flag.String("tag", "SPAMMER", "tag of txs")
var addr = flag.String("addr", strings.Repeat("9", 81), "the target address of the spam")
var msg = flag.String("msg", "", "the msg to send")
var valueBundles = flag.Bool("value", false, "spam value bundles")
var valueEntries = flag.Int("value-entries", 1, "value entries")
var valueSecLvl = flag.Int("value-sec-lvl", 2, "value sec level")

var targetAddr trinary.Hash
var emptySeed = strings.Repeat("9", 81)

func main() {
	flag.Parse()

	*addr = trinary.Pad(*addr, 81)

	var err error
	targetAddr, err = checksum.AddChecksum(*addr, true, consts.AddressChecksumTrytesSize)
	must(err)

	if len(*nodes) > 0 {
		split := strings.Split(*nodes, ",")
		for _, n := range split {
			for i := 0; i < *instancesNum; i++ {
				accSpammer(-1, n)
			}
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

const seedLength = 81

var tryteAlphabetLength = byte(len(consts.TryteAlphabet))

func GenerateSeed() (string, error) {
	var by [seedLength]byte
	if _, err := rand.Read(by[:]); err != nil {
		return "", err
	}
	var seed string
	for _, b := range by {
		seed += string(consts.TryteAlphabet[b%tryteAlphabetLength])
	}
	return seed, nil
}

func accSpammer(stopAfter int, nodeToUse ...string) {

	var powF pow.ProofOfWorkFunc

	if len(*powsrvioKey) > 0 {

		//powsrv.io config
		powClient := &powsrvio.PowClient{
			APIKey:        *powsrvioKey,
			ReadTimeOutMs: 5000,
			Verbose:       false,
		}

		// init powsrv.io
		err := powClient.Init()
		must(err)

		// use powsrv.io as pow func
		powF = powClient.PowFunc
	} else {
		_, powF = pow.GetFastestProofOfWorkImpl()

	}

	var n string
	if len(nodeToUse) > 0 {
		n = nodeToUse[0]
	} else {
		n = *node
	}

	iotaAPI, err := api.ComposeAPI(api.HTTPClientSettings{URI: n, LocalProofOfWorkFunc: powF})
	must(err)

	spamTransfer := []bundle.Transfer{{Address: targetAddr, Tag: *tag}}
	if len(*msg) > 0 {
		msgTrytes, err := converter.ASCIIToTrytes(*msg)
		must(err)
		spamTransfer[0].Message = msgTrytes
	}

	var bndl []trinary.Trytes
	if *valueBundles {
		seed, err := GenerateSeed()
		if err != nil {
			panic(err)
		}

		trnsf := []bundle.Transfer{}
		inputs := []api.Input{}
		for i := 0; i < *valueEntries; i++ {
			addr, err := address.GenerateAddress(seed, uint64(i), consts.SecurityLevel(*valueSecLvl), true)
			if err != nil {
				fmt.Printf("error creating address: %s\n", err.Error())
				panic(err)
			}
			trnsf = append(trnsf, bundle.Transfer{
				Address: addr,
				Tag:     *tag,
				Value:   100000000,
			})
			inputs = append(inputs, api.Input{
				Address:  addr,
				KeyIndex: uint64(i),
				Security: consts.SecurityLevel(*valueSecLvl),
				Balance:  100000000,
			})
		}

		bndl, err = PrepareTransfers(iotaAPI, seed, trnsf, api.PrepareTransfersOptions{Inputs: inputs,})
		if err != nil {
			fmt.Printf("error preparing transfer: %s\n", err.Error())
			panic(err)
		}
	} else {
		bndl, err = iotaAPI.PrepareTransfers(emptySeed, spamTransfer, api.PrepareTransfersOptions{})
		if err != nil {
			fmt.Printf("error preparing transfer: %s\n", err.Error())
			panic(err)
		}
	}

	go func() {
		for {

			tips, err := iotaAPI.GetTransactionsToApprove(uint64(*depth))
			if err != nil {
				fmt.Printf("error sending: %s\n", err.Error())
				continue
			}

			powedBndl, err := iotaAPI.AttachToTangle(tips.TrunkTransaction, tips.BranchTransaction, uint64(*mwm), bndl)
			if err != nil {
				fmt.Printf("error doing PoW: %s\n", err.Error())
				continue
			}

			_, err = iotaAPI.BroadcastTransactions(powedBndl...)
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

func getPrepareTransfersDefaultOptions(options api.PrepareTransfersOptions) api.PrepareTransfersOptions {
	if options.Security == 0 {
		options.Security = consts.SecurityLevelMedium
	}
	if options.Inputs == nil {
		options.Inputs = []api.Input{}
	}
	return options
}

func PrepareTransfers(apii *api.API, seed trinary.Trytes, transfers bundle.Transfers, opts api.PrepareTransfersOptions) ([]trinary.Trytes, error) {
	opts = getPrepareTransfersDefaultOptions(opts)

	if err := Validate(ValidateSeed(seed), ValidateSecurityLevel(opts.Security)); err != nil {
		return nil, err
	}

	for i := range transfers {
		if err := Validate(ValidateAddresses(transfers[i].Value != 0, transfers[i].Address)); err != nil {
			return nil, err
		}
	}

	var timestamp uint64
	txs := transaction.Transactions{}

	if opts.Timestamp != nil {
		timestamp = *opts.Timestamp
	} else {
		timestamp = uint64(time.Now().UnixNano() / int64(time.Second))
	}

	var totalOutput uint64
	for i := range transfers {
		totalOutput += transfers[i].Value
	}

	// add transfers
	outEntries, err := bundle.TransfersToBundleEntries(timestamp, transfers...)
	if err != nil {
		return nil, err
	}
	for i := range outEntries {
		txs = bundle.AddEntry(txs, outEntries[i])
	}

	// add input transactions
	var totalInput uint64
	for i := range opts.Inputs {
		if err := Validate(ValidateAddresses(opts.Inputs[i].Balance != 0, opts.Inputs[i].Address)); err != nil {
			return nil, err
		}
		totalInput += opts.Inputs[i].Balance
		input := &opts.Inputs[i]
		bndlEntry := bundle.BundleEntry{
			Address:   input.Address[:consts.HashTrytesSize],
			Value:     -int64(input.Balance),
			Length:    uint64(input.Security),
			Timestamp: timestamp,
		}
		txs = bundle.AddEntry(txs, bndlEntry)
	}

	// verify whether provided inputs fulfill threshold value
	if totalInput < totalOutput {
		return nil, consts.ErrInsufficientBalance
	}

	// finalize bundle by adding the bundle hash
	finalizedBundle, err := bundle.Finalize(txs)
	if err != nil {
		return nil, err
	}

	// compute signatures for all input txs
	normalizedBundleHash := signing.NormalizedBundleHash(finalizedBundle[0].Bundle)

	signedFrags := []trinary.Trytes{}
	for i := range opts.Inputs {
		input := &opts.Inputs[i]
		subseed, err := signing.Subseed(seed, input.KeyIndex)
		if err != nil {
			return nil, err
		}
		var sec consts.SecurityLevel
		if input.Security == 0 {
			sec = consts.SecurityLevelMedium
		} else {
			sec = input.Security
		}

		prvKey, err := signing.Key(subseed, sec)
		if err != nil {
			return nil, err
		}

		frags := make([]trinary.Trytes, input.Security)
		for i := 0; i < int(input.Security); i++ {
			signedFragTrits, err := signing.SignatureFragment(
				normalizedBundleHash[i*consts.HashTrytesSize/3:(i+1)*consts.HashTrytesSize/3],
				prvKey[i*consts.KeyFragmentLength:(i+1)*consts.KeyFragmentLength],
			)
			if err != nil {
				return nil, err
			}
			frags[i] = trinary.MustTritsToTrytes(signedFragTrits)
		}

		signedFrags = append(signedFrags, frags...)
	}

	// add signed fragments to txs
	var indexFirstInputTx int
	for i := range txs {
		if txs[i].Value < 0 {
			indexFirstInputTx = i
			break
		}
	}

	txs = bundle.AddTrytes(txs, signedFrags, indexFirstInputTx)

	// finally return built up txs as raw trytes
	return transaction.MustFinalTransactionTrytes(txs), nil
}
