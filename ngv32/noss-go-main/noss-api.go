package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cast"
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
	R_PSK_INIT   = `\.init\("([^"]+)"\)`
)

func GetBalance(pubkey string) (int, error) {
	address, _ := nip19.EncodePublicKey(pubkey)
	req, err := http.NewRequest(http.MethodGet, "https://api-worker.noscription.org/indexer/balance?npub="+address, nil)
	if err != nil {
		slog.Error("GetBalance", slog.String("err", err.Error()))
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
		slog.Error("GetBalance", slog.String("err", err.Error()))
		return -1, err
	}
	if resp.StatusCode != 200 {
		slog.Error("GetBalance", slog.String("err", "GetBalance"))
		return -1, fmt.Errorf("GetBalance error status code: %d", resp.StatusCode)
	}
	var result []map[string]interface{}
	defer resp.Body.Close()
	readBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("GetBalance read", slog.String("err", err.Error()))
	}
	err = json.Unmarshal(readBytes, &result)
	if err != nil {
		slog.Error("GetBalance parse", slog.String("err", err.Error()))
		return -1, err
	}
	if result == nil || len(result) == 0 {
		slog.Error("GetBalance parse", slog.String("err", "Erros is nill len(result) == 0"))
		return -1, fmt.Errorf("Erros is nill len(result) == 0")
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
				// 获取 PSK
				return match[1], nil
			}
		}
	}
	return "", err
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

//	{
//	    "id": "00000772623717b2142e085b37524e28b2cf6a3c7db0f2166ec21abb95ed45e9",
//	    "kind": 1,
//	    "created_at": 1704614893,
//	    "tags": [
//	        [
//	            "p",
//	            "9be107b0d7218c67b4954ee3e6bd9e4dba06ef937a93f684e42f730a0c3d053c"
//	        ],
//	        [
//	            "e",
//	            "51ed7939a984edee863bfbb2e66fdc80436b000a8ddca442d83e6a2bf1636a95",
//	            "wss://relay.noscription.org/",
//	            "root"
//	        ],
//	        [
//	            "e",
//	            "000002222c70688d7de6fb4673c5ec980caabd0bcff621cb19dcd4055b00ca5b",
//	            "wss://relay.noscription.org/",
//	            "reply"
//	        ],
//	        [
//	            "seq_witness",
//	            "167944670",
//	            "0xe986d7238d3cbd1fcb917efd3464a5603c5cbdb2e9372075a2eaa43c1113d685"
//	        ],
//	        [
//	            "nonce",
//	            "10mduor3uvp",
//	            "21"
//	        ]
//	    ],
//	    "content": "{\"p\":\"nrc-20\",\"op\":\"mint\",\"tick\":\"noss\",\"amt\":\"10\"}",
//	    "pubkey": "bb0bfafb5f5d436e98e4dafe786a6807eb0740f5453e8fcce16bba104018fa16"
//	}
func PostEvent(ev nostr.Event) error {
	evNewInstance := EV{
		Sig:       ev.Sig,
		Id:        ev.ID,
		Kind:      ev.Kind,
		CreatedAt: ev.CreatedAt,
		Tags:      ev.Tags,
		Content:   ev.Content,
		PubKey:    ev.PubKey,
	}
	// 将ev转为Json格式
	eventJSON, err := json.Marshal(evNewInstance)
	if err != nil {
		// log.Fatal(err)
		return err
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
		return err
	}

	req, err := http.NewRequest("POST", POST_SCRIPTION_URL, bytes.NewBuffer(wrapperJSON))
	if err != nil {
		//log.Fatalf("Error creating request: %v", err)
		fmt.Println("创建请求失败:", err.Error())
		return err
	}

	// 设置HTTP Header
	req.Header.Set("Sec-fetch-dest", "empty")
	req.Header.Set("Sec-fetch-mode", "cors")
	req.Header.Set("Sec-fetch-site", "same-site")
	req.Header.Set("Authority", "api-worker.noscription.org")
	req.Header.Set("origin", "https://noscription.org")
	req.Header.Set("Referer", "https://noscription.org/")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Dnt", "1")
	req.Header.Add("Sec-Ch-Ua", "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"120\"")
	req.Header.Add("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Add("Sec-Ch-Ua-Platform", "\"macOS\"")
	req.Header.Add("Sec-Fetch-Site", "same-origin")
	req.Header.Add("User-Agent", UA)
	startTime := time.Now()
	xgoron := GetGorgon(string(ev.ID))
	if xgoron == "" {
		//log.Fatalf("Error getting gorgon: %v", err)
		fmt.Println("获取gorgon失败:")
		return fmt.Errorf("Error getting gorgon: %v", err)
	}
	req.Header.Set("X-Gorgon", xgoron)
	// 发送请求
	resp, err := tlsclient.Do(req)
	fmt.Println("PostEvent time", time.Since(startTime))
	if err != nil {
		//log.Fatalf("Error sending request: %v", err)
		fmt.Println("发起请求失败:", err.Error())
		return err
	}
	if resp.StatusCode != 200 {
		//log.Fatalf("Error response status code: %v", resp.StatusCode)
		fmt.Println("Response Code Error:", resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			//log.Fatalf("Error reading response body: %v", err)
			fmt.Println("Read Response error :", err.Error())
			return err
		}
		if len(body) > 100 {
			body = body[:100]
		}
		fmt.Println("Response Body:", string(body))
		return fmt.Errorf("Error response status code: %v", resp.StatusCode)
	}
	defer resp.Body.Close()
  time.Sleep(1 * time.Second)
	return nil
}
