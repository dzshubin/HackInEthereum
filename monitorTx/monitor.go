package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"

	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	HashLength    = 32
	AddressLength = 20
)

// BytesToHash sets b to hash.
// If b is larger than len(h), b will be cropped from the left.
func BytesToHash(b []byte) common.Hash {
	var h common.Hash

	if len(b) > HashLength {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
	return h
}

func BytesToAddress(b []byte) common.Address {
	var h common.Address

	if len(b) > AddressLength {
		b = b[len(b)-AddressLength:]
	}

	copy(h[AddressLength-len(b):], b)
	return h
}

func GetHexStringBytes(s string) ([]byte, error) {
	if len(s) > 1 {
		if s[0:2] == "0x" || s[0:2] == "0X" {
			s = s[2:]

			hexBytes, _ := hex.DecodeString(s)
			return (hexBytes), nil

		} else {
			return []byte{}, fmt.Errorf("Not hex string!\n")
		}

	} else {
		return []byte{}, fmt.Errorf("Not hex string!\n")
	}
}

func HexStringToTxHash(s string) (common.Hash, error) {

	hexBytes, err := GetHexStringBytes(s)

	if err != nil {
		return [common.HashLength]byte{}, err
	} else {
		return BytesToHash(hexBytes), nil
	}
}

func HexStringToAddr(s string) (common.Address, error) {
	hexBytes, err := GetHexStringBytes(s)

	if err != nil {
		return [common.AddressLength]byte{}, err
	} else {
		return BytesToAddress(hexBytes), nil
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: monitor  [-address add] [-ws websocketUrl] 
Options:
`)
	flag.PrintDefaults()
}

func main() {

	websocketUrl := flag.String("ws", "wss://mainnet.infura.io/ws", "Websocket url")
	targetAddress := flag.String("address", "", "Your designated address")

	flag.Parse()

	if *targetAddress == "" {
		fmt.Println("Please designate a address YOU want to monitor.\n")
		printUsage()
		return
	}

	targetAddr, _ := HexStringToAddr(*targetAddress)

	rpccli, err := rpc.Dial(*websocketUrl)
	if err != nil {
		log.Fatalln(err)
	}

	ethc := ethclient.NewClient(rpccli)
	client := (*rpc.Client)(rpccli)
	subch := make(chan string, 1024)

	sub, err := client.EthSubscribe(context.Background(), subch, "newPendingTransactions")
	if err != nil {
		log.Fatalln(err)
	}

	abort := make(chan struct{})
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)

	go func(a chan<- struct{}) {

		defer signal.Stop(sigc)
		for {
			select {
			case <-sigc:
				close(a)
				return
			}
		}

	}(abort)

	txs := make(chan *types.Transaction, 1024)
	for {
		select {

		case <-abort:
			fmt.Printf("shutting down by outside...\n")
			return

		case hash := <-subch:
			bytesHash, err := HexStringToTxHash(hash)

			if err != nil {
				continue
			}

			go func(h common.Hash, results chan<- *types.Transaction) {
				tx, _, err := ethc.TransactionByHash(context.Background(), h)

				if err != nil {
					return
				} else {
					txs <- tx
				}
			}(bytesHash, txs)

		case err := <-sub.Err():
			log.Fatalln(err)
			return

		case tx := <-txs:
			var signer types.Signer = types.FrontierSigner{}
			if tx.Protected() {
				signer = types.NewEIP155Signer(tx.ChainId())
			}
			from, _ := types.Sender(signer, tx)

			// We've got a tx
			log.Printf("tx: 0x%x\n", tx.Hash())
			log.Printf("from: 0x%x\n", from)

			if bytes.Equal(targetAddr[:], from[:]) {
				go func(t *types.Transaction, client *ethclient.Client) {

					// we do something on it
					log.Println("<- We found a tx we want\n")
					Process(t, client)
				}(tx, ethc)
			}

		}
	}
}

func Process(t *types.Transaction, client *ethclient.Client) error {
	// We can do something evil if this specific tx sent by your designated address
	// for example, send a tx to inform someone

	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	from := crypto.PubkeyToAddress(key.PublicKey)

	nonce, err := client.NonceAt(context.Background(), from, nil)
	if err != nil {
		return err
	}

	to, _ := HexStringToAddr("0x003be5Df5FeF651EF0C59cD175c73ca1415f53eA")
	
	//send to mainnet
	signer := types.NewEIP155Signer(big.NewInt(1))
	tx := types.NewTransaction(nonce, to, big.NewInt(1000), params.TxGas, big.NewInt(1000000000), nil)
	tx, _ = types.SignTx(tx, signer, key)

	err = client.SendTransaction(context.Background(), tx)

	if err != nil {
		fmt.Printf("<- Sent tx failed.\n")
		return err
	}

	fmt.Printf("<- Execuate operation successfully.\n")
	fmt.Printf("<- from: %x, to: %x\n", from, tx.To)
	return nil
}
