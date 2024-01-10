package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"log/slog"
	"strings"

	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fuergaosi233/noss-go/cudalib"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/tidwall/gjson"
)

var sk string
var pk string
var numberOfWorkers int
var nonceFound int32 = 0
var blockNumber uint64
var hash atomic.Value
var messageId atomic.Value
var currentWorkers int32
var timeoutWorkers int32 = 0
var arbRpcUrl string
var signApi string
var postApi string
var wssAddr string
var keyApi string
var signKey atomic.Value
var arbRpcUrls []string

var (
	ErrDifficultyTooLow = errors.New("nip13: insufficient difficulty")
	ErrGenerateTimeout  = errors.New("nip13: generating proof of work took too long")
	ARBWebSocket        = "wss://arbitrum-one.public.blastapi.io/"
	PublicHeaders       = http.Header{
		"Pragma":          {"no-cache"},
		"Origin":          {"https://noscription.org"},
		"Accept-Language": {"en-US,en;q=0.9"},
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		"Cache-Control":   {"no-cache"},
	}
)

func init() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	sk = os.Getenv("sk")
	pk = os.Getenv("pk")
	numberOfWorkers, _ = strconv.Atoi(os.Getenv("numberOfWorkers"))
	arbRpcUrl = os.Getenv("arbRpcUrl")
	signApi = os.Getenv("signApi")
	postApi = os.Getenv("postApi")
	wssAddr = os.Getenv("wssAddr")
	keyApi = os.Getenv("keyApi")

	fmt.Println("signApi: ", signApi)
	fmt.Println("postApi: ", postApi)
	fmt.Println("arbRpcUrl: ", arbRpcUrl)
	fmt.Println("wssAddr: ", wssAddr)
	fmt.Println("keyApi: ", keyApi)

	arbRpcUrls = []string{
		"https://arbitrum.llamarpc.com",
		"https://rpc.arb1.arbitrum.gateway.fm",
		"https://api.zan.top/node/v1/arb/one/public",
		"https://arbitrum.meowrpc.com",
		"https://arb-pokt.nodies.app",
		"https://arbitrum.blockpi.network/v1/rpc/public",
		"https://arbitrum-one.publicnode.com",
		"https://arbitrum-one.public.blastapi.io",
		"https://arbitrum.drpc.org",
		"https://arb1.arbitrum.io/rpc",
		"https://endpoints.omniatech.io/v1/arbitrum/one/public",
		"https://1rpc.io/arb",
		"https://rpc.ankr.com/arbitrum",
		"https://arbitrum.api.onfinality.io/public",
		"wss://arbitrum-one.publicnode.com",
		"https://arb-mainnet-public.unifra.io",
		"https://arb-mainnet.g.alchemy.com/v2/demo",
		"https://arbitrum.getblock.io/api_key/mainnet",
	}

	// 签名key
	loadSignKey := os.Getenv("NOSS_SIGN_KEY")
	if loadSignKey == "" {
		loadSignKey = os.Getenv("signKey")
	}
	if loadSignKey != "" {
		signKey.Store(loadSignKey)
	}

	fmt.Println("NOSS_SIGN_KEY: ", loadSignKey)
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

func Generate(event nostr.Event, targetDifficulty int) (nostr.Event, error) {
	tag := nostr.Tag{"nonce", "", strconv.Itoa(targetDifficulty)}
	event.Tags = append(event.Tags, tag)
	start := time.Now()
	for {
		nonce, err := GenerateRandomString(10)
		if err != nil {
			fmt.Println(err)
		}
		tag[1] = nonce
		event.CreatedAt = nostr.Now()
		tempid := event.GetID()
		//fmt.Print("tempid:: ", tempid)
		if nip13.Difficulty(tempid) >= targetDifficulty {
			// fmt.Print(time.Since(start))
			/* fmt.Println(" ")
			fmt.Println("MintTime: ", time.Since(start)) */
			return event, nil
		}
		if time.Since(start) >= 1*time.Second {
			//fmt.Println("计算超时抛弃")
			atomic.AddInt32(&timeoutWorkers, 1)
			return event, ErrGenerateTimeout
		}
	}
}
func GenerateWithCuda(event nostr.Event, targetDifficulty int) (nostr.Event, error) {

	tag := nostr.Tag{"nonce", "", strconv.Itoa(targetDifficulty)}
	event.Tags = append(event.Tags, tag)
	start := time.Now()
	for {
		event.CreatedAt = nostr.Now()
		tag[1] = "0123456789123"
		inputData := event.Serialize()
		nonce := cudalib.SolveNoss(string(inputData), targetDifficulty/4)
		tag[1] = nonce
		//fmt.Print("tempid:: ", tempid)
		if nip13.Difficulty(event.GetID()) >= targetDifficulty {
			// fmt.Print(time.Since(start))
			/* fmt.Println(" ")
			fmt.Println("MintTime: ", time.Since(start)) */
			return event, nil
		}
		if time.Since(start) >= 1*time.Second {
			fmt.Println("计算超时抛弃")
			atomic.AddInt32(&timeoutWorkers, 1)
			return event, ErrGenerateTimeout
		}
	}
}

type Message struct {
	EventId string `json:"eventId"`
}

type EV struct {
	Sig       string          `json:"sig"`
	Id        string          `json:"id"`
	Kind      int             `json:"kind"`
	CreatedAt nostr.Timestamp `json:"created_at"`
	Tags      nostr.Tags      `json:"tags"`
	Content   string          `json:"content"`
	PubKey    string          `json:"pubkey"`
}

// get html
func GetCurlBody(url string) ([]byte, error) {
	var failRes = []byte("")
	res, err := http.Get(url)
	if err != nil {
		return failRes, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return failRes, err
	}
	return body, nil
}

// client *ethclient.Client
func mine(ctx context.Context, _messageId string, _blockNumber uint64, _hash string, _signKey string) {
	//
	fmt.Println("Start Mint")
	replayUrl := "wss://relay.noscription.org/"
	difficulty := 21

	// Create a channel to signal the finding of a valid nonce
	foundEvent := make(chan nostr.Event, 1)
	notFound := make(chan nostr.Event, 1)
	// Create a channel to signal all workers to stop
	//content := `{"p":"nrc-20","op":"mint","tick":"noss","amt":"10"}`
	content := "{\"p\":\"nrc-20\",\"op\":\"mint\",\"tick\":\"noss\",\"amt\":\"10\"}"

	//content := `{\"p\":\"nrc-20\",\"op\":\"mint\",\"tick\":\"noss\",\"amt\":\"10\"}`
	startTime := time.Now()

	var mintTime time.Duration

	ev := nostr.Event{
		Content:   content,
		CreatedAt: nostr.Now(),
		ID:        "",
		Kind:      nostr.KindTextNote,
		PubKey:    pk,
		Sig:       "",
		Tags:      nil,
	}
	ev.Tags = ev.Tags.AppendUnique(nostr.Tag{"p", "9be107b0d7218c67b4954ee3e6bd9e4dba06ef937a93f684e42f730a0c3d053c"})
	ev.Tags = ev.Tags.AppendUnique(nostr.Tag{"e", "51ed7939a984edee863bfbb2e66fdc80436b000a8ddca442d83e6a2bf1636a95", replayUrl, "root"})
	ev.Tags = ev.Tags.AppendUnique(nostr.Tag{"e", _messageId, replayUrl, "reply"})
	ev.Tags = ev.Tags.AppendUnique(nostr.Tag{"seq_witness", strconv.Itoa(int(_blockNumber)), _hash})
	// Start multiple worker goroutines
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			evCopy := ev
			evCopy, err := GenerateWithCuda(evCopy, difficulty)
			if err != nil {
				// fmt.Println(err)
				/* atomic.AddInt32(&currentWorkers, -1)
				return */
				notFound <- evCopy
			}
			mintTime = time.Since(startTime)
			foundEvent <- evCopy
		}
	}()

	select {
	case <-notFound:
	case evNew := <-foundEvent:
		evNew.Sign(sk)

		evNewInstance := EV{
			Sig:       evNew.Sig,
			Id:        evNew.ID,
			Kind:      evNew.Kind,
			CreatedAt: evNew.CreatedAt,
			Tags:      evNew.Tags,
			Content:   evNew.Content,
			PubKey:    evNew.PubKey,
		}
		// 将ev转为Json格式
		eventJSON, err := json.Marshal(evNewInstance)
		if err != nil {
			// log.Fatal(err)
			return
		}

		wrapper := map[string]json.RawMessage{
			"event": eventJSON,
		}

		// 将包装后的对象序列化成JSON
		/* wrapperJSON, err := json.MarshalIndent(wrapper, "", "  ") // 使用MarshalIndent美化输出
		if err != nil {
			log.Fatalf("Error marshaling wrapper: %v", err)
		} */

		wrapperJSON, err := json.Marshal(wrapper)
		if err != nil {
			//log.Fatalf("Error marshaling wrapper: %v", err)
			return
		}

		req, err := http.NewRequest("POST", postApi, bytes.NewBuffer(wrapperJSON))
		if err != nil {
			//log.Fatalf("Error creating request: %v", err)
			fmt.Println("创建请求失败:", err.Error())
			return
		}

		// 设置HTTP Header
		req.Header.Set("Sec-ch-ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
		req.Header.Set("Sec-ch-ua-mobile", "?0")
		req.Header.Set("Sec-ch-ua-platform", "macOS")
		req.Header.Set("Sec-fetch-dest", "empty")
		req.Header.Set("Sec-fetch-mode", "cors")
		req.Header.Set("Sec-fetch-site", "same-site")
		req.Header.Set("Authority", "api-worker.noscription.org")
		req.Header.Set("origin", "https://noscription.org")
		req.Header.Set("Referer", "https://noscription.org/")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		// 存在签名接口，在本机签名
		if _signKey != "" {
			tempSignUrl := signApi + "?id=" + evNew.ID + "&key=" + _signKey
			antiSign, err := GetCurlBody(tempSignUrl)
			if err != nil {
				//log.Fatalf("Error sign request: %v", err)
				fmt.Println("本地nodejs计算签名:", err.Error())
				return
			}
			tempAntiSignValueStr := string(antiSign)
			req.Header.Set("X-Gorgon", tempAntiSignValueStr)
		} else {
			// 不存在签名接口，key传到节点服务器签名
			req.Header.Set("user-token", _signKey)
		}

		// 发送请求
		http_client := &http.Client{}
		resp, err := http_client.Do(req)
		if err != nil {
			//log.Fatalf("Error sending request: %v", err)
			fmt.Println("发起请求失败:", err.Error())
			return
		}
		defer resp.Body.Close()

		spendTime := time.Since(startTime)
		//fmt.Println("Response Status:", resp.Status)
		fmt.Println(" ")
		if resp.Status != "200 OK" {
			body, _ := ioutil.ReadAll(resp.Body)
			fmt.Println("❌", "mine:", mintTime, " spend:", spendTime, " id:", evNew.ID, " block:", _blockNumber, " timeout:", atomic.LoadInt32(&timeoutWorkers))
			fmt.Println(resp.Status)
			if resp.Status != "403 Forbidden" {
				fmt.Println(string(body))
			}
			fmt.Println("Data:", string(wrapperJSON))
			fmt.Println(nostr.Now().Time())
			fmt.Println("signkey:", _signKey)
		} else {
			fmt.Println("✅", "mine:", mintTime, " spend:", spendTime, " id:", evNew.ID, " block:", _blockNumber, " timeout:", atomic.LoadInt32(&timeoutWorkers))
			fmt.Println(nostr.Now().Time())
			fmt.Println("signkey:", _signKey)
		}

		atomic.StoreInt32(&nonceFound, 0)
	case <-ctx.Done():
		fmt.Print("done")
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

	slog.Info("Connect Noscription Success")
	startTime := time.Now()
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
		messageId.Store(eventID)
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
				hash.Store(header.Hash().Hex())
				atomic.StoreUint64(&blockNumber, header.Number.Uint64())
			}
		}
	}
}

func main() {

	// wssAddr := "wss://report-worker-2.noscription.org"
	// relayUrl := "wss://relay.noscription.org/"
	godotenv.Load()
	ctx := context.Background()

	/* var err error
	client, err := ethclient.Dial(arbRpcUrl)
	if err != nil {
		//log.Fatalf("无法连接到Arbitrum节点: %v", err)
		fmt.Println("无法连接到Arbitrum节点:", err.Error())
		return
	} */

	// initialize an empty cancel function

	// get block
	go ListenARBLastBlock()
	go GetLastEventID()
	go func() {
		for {
			// 从url获取key
			keyResult, err := GetCurlBody(keyApi)
			keyResultStr := ""
			if err == nil {
				keyResultStr = string(keyResult)
			}
			if keyResultStr != "" {
				signKey.Store(keyResultStr)
			}
			// 添加1分钟的延迟
			time.Sleep(1 * time.Minute)
		}
	}()

	atomic.StoreInt32(&currentWorkers, 0)
	//atomic.StoreInt32(&timeoutWorkers, 0)
	// 初始化一个取消上下文和它的取消函数
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听blockNumber和messageId变化
	go func() {
		for {
			select {
			case <-ctx.Done(): // 如果上下文被取消，则退出协程
				return
			default:
				if atomic.LoadInt32(&currentWorkers) < int32(numberOfWorkers) && messageId.Load() != nil && signKey.Load() != nil && atomic.LoadUint64(&blockNumber) > 0 {
					atomic.AddInt32(&currentWorkers, 1) // 增加工作者数量
					//currentBlockNumber := atomic.LoadUint64(&blockNumber)
					go func(bn uint64, mid string, hs string, sk string) {
						defer atomic.AddInt32(&currentWorkers, -1) // 完成后减少工作者数量
						mine(ctx, mid, bn, hs, sk)
					}(atomic.LoadUint64(&blockNumber), messageId.Load().(string), hash.Load().(string), signKey.Load().(string))
				}
			}
		}
	}()
	fmt.Printf("NumberOfWorkers: %d\n", numberOfWorkers)
	select {}

}
