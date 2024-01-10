package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-resty/resty/v2"
	"github.com/gorilla/websocket"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cast"
	"github.com/tidwall/gjson"
)

var (
	client              = resty.New().SetTimeout(20 * time.Second)
	difficulty          = 5
	lastEventID         string
	ArbBlockNumber      string
	ArbBlockHash        string
	lastCommitID        string
	success_times       = 0
	CalculationInterval = 1000
	PublicTag           = make(nostr.Tags, 0)
	ChangeSingleChan    = make(chan string, 20)

	CommitCache, _ = lru.New[string, bool](100)
	LastEvent, _   = lru.New[string, bool](100)
	PublicHeaders  = http.Header{
		"Pragma":          {"no-cache"},
		"Origin":          {"https://noscription.org"},
		"Accept-Language": {"en-US,en;q=0.9"},
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		"Cache-Control":   {"no-cache"},·
	}
	commit_queue = make(chan bool, 1000)
	XGORGON_URL  = "http://localhost:3000/gorgon"
	PSK          = ""
	Wallets      []*Wallet
	LARK_WEBHOOK = ""
)
var (
	WebsocketConnectCount    = 20
	WebsocketConnectInterval = 2000 // MS

)
var (
	ARBWebSocket         = "wss://arbitrum-one.public.blastapi.io/"
	ARBQueue             = make(chan bool, 1000)
	ARBWebSocketInterval = 1 // MS
)
var (
	ResubmitQueue      = make(chan bool, 1000)
	SubmitTimeInterval = 300 // MS
)
var (
	NOSTR_PRIVATE_KEYS = ""
)

func GetALLBalance() int64 {
	balance := int64(0)
	for _, wallet := range Wallets {
		if wallet.Balance > 0 {
			balance += int64(wallet.Balance)
		}
	}
	return balance
}
func WorkerStat() {
	interval := 10
	tick := time.Tick(time.Duration(interval) * time.Second)
	commit_times := 0
	recommit_times := 0
	last_blance := int64(0)
	wallet_count := len(Wallets)
	for {
		select {
		case <-tick:
			new_balance := GetALLBalance()
			if new_balance != last_blance {
				fmt.Printf("Balance Change %d \n current Blance is %d", new_balance-last_blance, new_balance)
				if LARK_WEBHOOK != "" {
					go SendLarkMessage(LARK_WEBHOOK, fmt.Sprintf("Balance Change %d \n current Blance is %d", new_balance-last_blance, new_balance))
				}
				last_blance = new_balance
			}
			fmt.Printf("Commit Speed: %.2f /s \n", float64(commit_times)/float64(interval))
			fmt.Println("PSK is ", PSK)
			fmt.Printf("ReCommit Speed: %.2f /s \n", float64(recommit_times)/float64(interval))
			fmt.Printf("Balance: %d \n", GetALLBalance())
			fmt.Printf("ARBBlockNumber: %s \n", ArbBlockNumber)
			fmt.Printf("ARBBlockHash: %s \n", ArbBlockHash)
			fmt.Println("Wallet Count", wallet_count)
			fmt.Printf("")
			commit_times = 0
			recommit_times = 0
		case <-commit_queue:
			commit_times += 1
		case <-ResubmitQueue:
			recommit_times += 1
		}
	}

}
func main() {
	godotenv.Load()
	privateKey := os.Getenv("NostrPrivateKey")
	if os.Getenv("ARB_WEBSOCKET") != "" {
		ARBWebSocket = os.Getenv("ARB_WEBSOCKET")
	}
	if os.Getenv("XGORGON_URL") != "" {
		XGORGON_URL = os.Getenv("XGORGON_URL")
	}
	if os.Getenv("LARK_WEBHOOK") != "" {
		LARK_WEBHOOK = os.Getenv("LARK_WEBHOOK")
	}
	if os.Getenv("NOSTR_PRIVATE_KEYS") != "" {
		NOSTR_PRIVATE_KEYS = os.Getenv("NOSTR_PRIVATE_KEYS")
	}
	if strings.HasPrefix(privateKey, "nsec") {
		_, v, err := nip19.Decode(privateKey)
		if err != nil {
			fmt.Printf("error "+"nost Decode", slog.String("err", err.Error()))
			panic("nost Decode error")
		}
		privateKey = cast.ToString(v)
	}
	var err error
	fmt.Println("Start Mine")
	for PSK == "" {
		PSK, err = GetPSKValue()
		if err != nil {
			fmt.Println("GetPSK Error, Retry", err.Error())
		}
		time.Sleep(1 * time.Second)
	}
	// Init wallet
	{
		if NOSTR_PRIVATE_KEYS != "" {
			// Split by ,
			privateKeys := strings.Split(NOSTR_PRIVATE_KEYS, ",")
			for _, privateKey := range privateKeys {
				if strings.HasPrefix(privateKey, "nsec") {
					_, v, err := nip19.Decode(privateKey)
					if err != nil {
						fmt.Printf("error "+"nost Decode", slog.String("err", err.Error()))
						return
					}
					privateKey = cast.ToString(v)
				}
				wallet, err := NewWallet(privateKey)
				if err != nil {
					fmt.Printf("error "+"NewWallet", slog.String("err", err.Error()))
					return
				}
				Wallets = append(Wallets, wallet)
				time.Sleep(0.2 * time.Second)
			}
		} else {
			wallet, err := NewWallet(privateKey)
			if err != nil {
				fmt.Println("Error NewWallet", slog.String("err", err.Error()))
				return
			}
			Wallets = append(Wallets, wallet)
		}
	}
	if len(NOSTR_PRIVATE_KEYS) == 0 {
		slog.Error("NOSTR_PRIVATE_KEYS is empty")
		return
	}
	fmt.Println("Set PSK", PSK)
	go StartWork()
	go UpdatePSKKey()
	go ListenARBLastBlock()
	go WorkerStat()
	go RefreshAllBalance()
	for i := 0; i < WebsocketConnectCount; i++ {
		time.Sleep(2000 * time.Millisecond)
		go GetLastEventID()
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
	os.Exit(0)
}
func RefreshAllBalance() {
	for {
		for _, wallet := range Wallets {
			_, err := wallet.GetBalance()
			if err != nil {
				slog.Error("RefreshAllBalance", slog.String("err", err.Error()))
			}
			time.Sleep(5 * time.Second)
		}
	}
}
func GetLastEventID() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, response, err := websocket.DefaultDialer.DialContext(ctx, "wss://report-worker-2.noscription.org/", PublicHeaders)
	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			time.Sleep(1 * time.Second)
			go GetLastEventID()
			return
		}
		if response != nil && response.StatusCode == 429 {
			go GetLastEventID()
			return
		}
		if response != nil {
			slog.Error("noscription websocket", "err", err.Error(), "response status", response.StatusCode)
		}
		go GetLastEventID()
		return
	}
	defer conn.Close()

	fmt.Println("Connect Noscription Success")
	startTime := time.Now()
	lastEventID := ""
	for {
		if time.Now().Sub(startTime) > 1*time.Minute {
			fmt.Println("Reconnect Noscription")
			go GetLastEventID()
			return
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Error("noscription websocket", slog.String("err", err.Error()))
			go GetLastEventID()
			return
		}
		eventID := gjson.Get(string(message), "eventId").String()
		if eventID != "" {
			if eventID != lastEventID {
				fmt.Printf("Get LastEvent ID %s \n", eventID)
				ChangeSingleChan <- eventID
			}
			lastEventID = eventID
			if lastCommitID == lastEventID {
				fmt.Println("🎉 Hit the target")
				success_times += 1
			}
		}
		time.Sleep(0.3 * time.Second)
	}
}
func ListenARBLastBlock() {
	headers := make(chan *types.Header)
	for {
		client, err := ethclient.Dial(ARBWebSocket)
		if err != nil {
			slog.Error("arb client websocket", slog.String("err", err.Error()))
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		sub, err := client.SubscribeNewHead(context.Background(), headers)
		if err != nil {
			slog.Error("arb client SubscribeNewHead", slog.String("err", err.Error()))
			continue
		}
		for {
			select {
			case err := <-sub.Err():
				slog.Info("arb client websocket", slog.String("err", err.Error()))
				break
			case header := <-headers:
				newArbBlockHash := header.Hash().Hex()
				newArbBlockNumber := header.Number.String()
				if newArbBlockNumber != ArbBlockNumber && newArbBlockNumber != "0" {
					ArbBlockNumber = newArbBlockNumber
					ArbBlockHash = newArbBlockHash
					// ARBQueue <- true
				}
			}
		}
	}
}
func GetArbLatestBlock() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsLock := sync.RWMutex{}

	conn, response, err := websocket.DefaultDialer.DialContext(ctx, ARBWebSocket, nil)
	if err != nil {
		fmt.Print("arb websocket", slog.String("err", err.Error()))
		if response != nil {
			slog.Error("arb websocket", "err", err.Error(), "response status", response.StatusCode)
			responseBody, _ := io.ReadAll(response.Body)
			fmt.Println(string(responseBody))
			// Print Result
		}
		go GetArbLatestBlock()
		return
	}
	defer conn.Close()

	slog.Info("Connect arbitrum success")

	done := make(chan struct{})
	defer close(done)

	t := time.NewTicker(time.Duration(ARBWebSocketInterval) * time.Millisecond)
	defer t.Stop()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-t.C:
				wsLock.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte(`{"method":"eth_blockNumber","id":1,"jsonrpc":"2.0"}`))
				wsLock.Unlock()
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Error("arb websocket", slog.String("err", err.Error()))
			go GetArbLatestBlock()
			return
		}

		item := gjson.ParseBytes(message)
		switch item.Get("id").Int() {
		case 1:
			number := item.Get("result").String()

			wsLock.Lock()
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"method":"eth_getBlockByNumber","params":["%s",false],"id":2,"jsonrpc":"2.0"}`, number)))
			wsLock.Unlock()
		case 2:
			numStr := gjson.Get(string(message), "result.number").String()
			n := new(big.Int)
			n.SetString(strings.TrimPrefix(numStr, "0x"), 16)

			newArbBlockNumber := n.String()
			newArbBlockHash := gjson.Get(string(message), "result.hash").String()
			if newArbBlockNumber != ArbBlockNumber && newArbBlockNumber != "0" {
				ArbBlockNumber = newArbBlockNumber
				ArbBlockHash = newArbBlockHash
				ARBQueue <- true
			}
		}
	}
}
func StartWork() {
	current_event_id := ""
	for {
		select {
		case eventId := <-ChangeSingleChan:
			_, ok := LastEvent.Get(eventId)
			if ok {
				continue
			} else {
				LastEvent.Add(eventId, true)
			}
			current_event_id = eventId
			for _, wallet := range Wallets {
				go wallet.Mine(current_event_id)
			}
		case <-ARBQueue:
			for _, wallet := range Wallets {
				go wallet.Mine(current_event_id)
			}
		}
	}
}

//	func WorkForever(ctx context.Context, eventId string) {
//		r := rand.New(rand.NewSource(time.Now().UnixNano()))
//		if arbBlockHash == "" || ArbBlockNumber == "" || eventId == "" {
//			fmt.Printf("NO data to solve\n")
//		}
//		ev := composeEvent(r, eventId)
//		ev.PubKey = PubKey
//		inputData := ev.Serialize()
//		for {
//			select {
//			case <-ctx.Done():
//				// fmt.Printf("Stop work for \n", eventId)
//				return
//			default:
//				start_time := time.Now()
//				nonce := SolveNoss(string(inputData), difficulty)
//				ev.Tags[4][1] = nonce
//				err, value := check_ev_valid(&ev)
//				if err != nil {
//					fmt.Printf("Mine error", err.Error(), "\n")
//				}
//				if !value {
//					fmt.Printf("Mine error result\n")
//				}
//				slog.Info("Cost Time", slog.String("time", time.Now().Sub(start_time).String()))
//				go postEvent(ev)
//			}
//			time.Sleep(time.Duration(SubmitTimeInterval) * time.Millisecond)
//		}
//	}
// func Work(ctx context.Context, eventId string) {
// 	r := rand.New(rand.NewSource(time.Now().UnixNano()))
// 	if arbBlockHash == "" || ArbBlockNumber == "" || eventId == "" {
// 		fmt.Printf("NO data to solve\n")
// 	}
// 	ev := composeEvent(r, eventId)
// 	ev.PubKey = PubKey
// 	inputData := ev.Serialize()
// 	start_time := time.Now()
// 	nonce := SolveNoss(string(inputData), difficulty)
// 	ev.Tags[4][1] = nonce
// 	err, value := check_ev_valid(&ev)
// 	if err != nil {
// 		fmt.Printf("Mine error", err.Error(), "\n")
// 	}
// 	if !value {
// 		fmt.Printf("Mine error result\n")
// 	}
// 	slog.Info("Cost Time", slog.String("time", time.Now().Sub(start_time).String()))
// 	go postEvent(ev)

// }

func postEvent(ev nostr.Event) {
	// slog.Info("Get Gorgon Success", slog.String("xgorgon", xgorgon))
	for i := 0; i < 3; i++ {
		_, ok := CommitCache.Get(ev.ID)
		if ok {
			ResubmitQueue <- true
			return
		}
		CommitCache.Add(ev.ID, true)
		err := PostEvent(ev)
		if err != nil {
			slog.Error("PostEvent", slog.String("err", err.Error()))
			continue
		}
		commit_queue <- true
		break
	}
}
