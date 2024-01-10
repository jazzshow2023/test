package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"regexp"
	"time"

	nhttp "net/http"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-resty/resty/v2"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cast"
)

var (
	client              = resty.New().SetTimeout(20 * time.Second)
	difficulty          = 21
	lastEventID         string
	arbBlockNumber      string
	arbBlockHash        string
	lastCommitID        string
	sk                  *secp256k1.PrivateKey
	pk                  *secp256k1.PublicKey
	PubKey              string
	success_times       = 0
	CalculationInterval = 1000
	WebsocketDialer     = 0
	uProxy, _           = url.Parse("")
	LARK_WEBHOOK        = ""
)
var (
	jar     = tls_client.NewCookieJar()
	options = []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(360),
		tls_client.WithClientProfile(profiles.Chrome_112),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCookieJar(jar), // create cookieJar instance and pass it as argument
		// Disable SSL verification
		tls_client.WithInsecureSkipVerify(),
	}
	tlsclient, _ = tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	UA           = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36"
	R_HOME_JS    = `/_next/static/chunks/pages/[^"]+`
	R_PSK_INIT   = `eT\.init\("([^"]+)"\)`
)

func getBlanace(pubkey string) (int, error) {
	address, _ := nip19.EncodePublicKey(pubkey)
	req, err := http.NewRequest(http.MethodGet, "https://api-worker.noscription.org/indexer/balance?npub="+address, nil)
	if err != nil {
		slog.Error("getBlanace", slog.String("err", err.Error()))
	}
	req.Header = http.Header{

		"Authority":          {"api-worker.noscription.org"},
		"Accept":             {"application/json, text/plain, */*"},
		"Accept-Language":    {"en-US,en;q=0.9"},
		"Dnt":                {"1"},
		"Origin":             {"https://noscription.org"},
		"Referer":            {"https://noscription.org/"},
		"Sec-Ch-Ua":          {"\"Not_A Brand\";v=\"8\": \"Chromium\";v=\"120\""},
		"Sec-Ch-Ua-Mobile":   {"?0"},
		"Sec-Ch-Ua-Platform": {"\"macOS\""},
		"Sec-Fetch-Dest":     {"empty"},
		"Sec-Fetch-Mode":     {"cors"},
		"Sec-Fetch-Site":     {"same-site"},
		"User-Agent":         {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
	}
	resp, err := tlsclient.Do(req)
	if err != nil {
		slog.Error("getBlanace", slog.String("err", err.Error()))
		return -1, err
	}
	if resp.StatusCode != 200 {
		slog.Error("getBlanace", slog.String("err", "getBlanace"))
		return -1, fmt.Errorf("getBlanace error status code: %d", resp.StatusCode)
	}
	var result []map[string]interface{}
	defer resp.Body.Close()
	readBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("getBlanace read", slog.String("err", err.Error()))
	}
	err = json.Unmarshal(readBytes, &result)
	if err != nil {
		slog.Error("getBlanace parse", slog.String("err", err.Error()))
		return -1, err
	}
	if len(result) == 0 {
		slog.Error("getBlanace parse", slog.String("err", "len(result) == 0"))
	}
	return cast.ToInt(result[0]["balance"]), nil

}
func GetHomeJS() ([]string, error) {
	// Create request
	req, err := http.NewRequest("GET", "https://noscription.org/???", nil)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return nil, err
	}
	// Headers
	req.Header.Add("Authority", "noscription.org")
	req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9")
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Dnt", "1")
	req.Header.Add("Pragma", "no-cache")
	req.Header.Add("Sec-Ch-Ua", "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"120\"")
	req.Header.Add("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Add("Sec-Ch-Ua-Platform", "\"macOS\"")
	req.Header.Add("Sec-Fetch-Dest", "document")
	req.Header.Add("Sec-Fetch-Mode", "navigate")
	req.Header.Add("Sec-Fetch-Site", "same-origin")
	req.Header.Add("Sec-Fetch-User", "?1")
	req.Header.Add("Upgrade-Insecure-Requests", "1")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	// Fetch Request
	resp, err := tlsclient.Do(req)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return nil, err
	}
	if resp.StatusCode != 200 {
		slog.Error("GetPSK", slog.String("err", "GetPSK"))
		return nil, fmt.Errorf("GetPSK error status code: %d", resp.StatusCode)
	}
	// Read Response Body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return nil, err
	}
	// Display Results
	html := string(body)
	re := regexp.MustCompile(R_HOME_JS)
	matches := re.FindAllStringSubmatch(html, -1)
	var result []string
	for _, match := range matches {
		result = append(result, match[0])
	}
	return result, nil
}
func GetPSK(url string) (string, error) {
	// Create request
	req, err := http.NewRequest("GET", "https://noscription.org"+url, nil)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return "", err
	}
	// Headers
	req.Header.Add("Authority", "noscription.org")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Dnt", "1")
	req.Header.Add("Sec-Ch-Ua", "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"120\"")
	req.Header.Add("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Add("Sec-Ch-Ua-Platform", "\"macOS\"")
	req.Header.Add("Sec-Fetch-Dest", "script")
	req.Header.Add("Sec-Fetch-Mode", "no-cors")
	req.Header.Add("Sec-Fetch-Site", "same-origin")
	req.Header.Add("User-Agent", UA)
	// Fetch Request
	resp, err := tlsclient.Do(req)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return "", err
	}
	if resp.StatusCode != 200 {
		slog.Error("GetPSK", slog.String("err", "GetPSK"))
		return "", fmt.Errorf("GetPSK error status code: %d", resp.StatusCode)
	}
	// Read Response Body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return "", err
	}
	// Display Results
	html := string(body)
	re := regexp.MustCompile(R_PSK_INIT)
	matches := re.FindAllStringSubmatch(html, -1)

	if matches != nil {
		for _, match := range matches {
			if len(match) > 1 {
				return match[1], nil
			}
		}
	}
	return "", err
}

// {"msg_type":"text","content":{"text":"request example"}}

type Message struct {
	Msgtype string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

// sendFeishuMessage sends a message to the specified Feishu hook
func sendFeishuMessage(url string, text string) error {
	contentType := "application/json"

	// Construct the JSON payload
	payload := map[string]interface{}{
		"content": map[string]string{
			"text": text,
		},
		"msg_type": "text",
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Send the POST request
	resp, err := nhttp.Post(url, contentType, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Add additional response handling here if needed

	return nil
}

func main() {
	client, err := ethclient.Dial("wss://arb-mainnet.g.alchemy.com/v2/odIVXihl8fN7X_EB25TNumja2viJblAp")
	if err != nil {
		log.Fatal(err)
	}

	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case err := <-sub.Err():
			log.Fatal(err)
		case header := <-headers:
			fmt.Println(header.Hash().Hex()) // 0xbc10defa8dda384c96a17640d84de5578804945d347072e091b4e5f390ddea7f
			fmt.Println(header.Number.Uint64())
		}
	}
}
