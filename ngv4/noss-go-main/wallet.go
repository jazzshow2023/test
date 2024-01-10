package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/fuergaosi233/noss-go/cudalib"
	"github.com/nbd-wtf/go-nostr"
)

// TODO: Add a comment here
type Wallet struct {
	PrivateKey string
	PublicKey  string
	Sk         *secp256k1.PrivateKey
	Pk         *secp256k1.PublicKey
	Balance    int
}

func NewWallet(
	privateKey string,
) (*Wallet, error) {
	w := &Wallet{
		PrivateKey: privateKey,
	}
	_, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		slog.Error("nost GetPublicKey", slog.String("err", err.Error()))
		return nil, err
	}
	s, err := hex.DecodeString(privateKey)
	if err != nil {
		panic(fmt.Errorf("Sign called with invalid private key '%s': %w", privateKey, err))
	}

	sk, pk := btcec.PrivKeyFromBytes(s)
	w.Sk = sk
	w.Pk = pk
	pkBytes := pk.SerializeCompressed()
	PubKey := hex.EncodeToString(pkBytes[1:])
	w.PublicKey = PubKey
	return w, nil
}
func (w *Wallet) GetBalance() (int, error) {
	balance, err := GetBalance(w.PublicKey)
	if err != nil {
		slog.Error("GetBalance", slog.String("err", err.Error()))
	}
	if balance != -1 {
		w.Balance = balance
	}
	return balance, err
}
func GenerateRandomString(length int) (string, error) {
	charset := "abcdefghijklmnopqrstuvwxyz0123456789" // 字符集
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	for i := 0; i < length; i++ {
		b[i] = charset[int(b[i])%len(charset)]

	}

	return string(b), nil
}
func (*Wallet) GenerateEvent(
	eventId string,
) nostr.Event {
	nonce, _ := GenerateRandomString(10)

	ev := nostr.Event{}
	ev.Kind = nostr.KindTextNote
	ev.CreatedAt = nostr.Now()
	ev.Content = `{"p":"nrc-20","op":"mint","tick":"noss","amt":"10"}`
	ev.Tags = nostr.Tags{
		{
			`p`,
			`9be107b0d7218c67b4954ee3e6bd9e4dba06ef937a93f684e42f730a0c3d053c`,
		},
		{
			`e`,
			`51ed7939a984edee863bfbb2e66fdc80436b000a8ddca442d83e6a2bf1636a95`,
			`wss://relay.noscription.org/`,
			`root`,
		},
		{
			`e`,
			eventId,
			`wss://relay.noscription.org/`,
			`reply`,
		},
		{
			"seq_witness",
			ArbBlockNumber,
			ArbBlockHash,
		},
		{
			"nonce",
			nonce,
			"21",
		},
	}
	return ev
}
func (w *Wallet) check_ev_valid(evt *nostr.Event) (error, bool) {
	h := sha256.Sum256(evt.Serialize())
	evt.ID = hex.EncodeToString(h[:])
	if !strings.HasPrefix(evt.ID, "00000") {
		return nil, false
	}
	sig, err := schnorr.Sign(w.Sk, h[:])
	if err != nil {
		return fmt.Errorf("Sign error" + err.Error()), false
	}
	evt.Sig = hex.EncodeToString(sig.Serialize())

	return err, true
}
func (w *Wallet) Mine(eventId string) {
	if ArbBlockHash == "" || ArbBlockNumber == "" || eventId == "" {
		fmt.Println("ArbBlockHash", ArbBlockHash)
		fmt.Println("ArbBlockNumber", ArbBlockNumber)
		fmt.Println("eventId", eventId)
		fmt.Printf("NO data to solve\n")
		return
	}
	ev := w.GenerateEvent(eventId)
	ev.PubKey = w.PublicKey
	inputData := ev.Serialize()
	start_time := time.Now()
	nonce := cudalib.SolveNoss(string(inputData), difficulty)
	fmt.Println("Mine time", time.Since(start_time))
	ev.Tags[4][1] = nonce

	err, value := w.check_ev_valid(&ev)
	if err != nil {
		fmt.Print("Mine error", err.Error(), "\n")
	}
	if !value {
		fmt.Printf("Mine error result %s \n", nonce)
		return
	}
	go postEvent(ev)
}
