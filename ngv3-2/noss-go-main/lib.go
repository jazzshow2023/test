package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func check(e string) int {
	t := 0
	for i := 0; i < len(e); i++ {
		r := e[i]
		n, err := strconv.ParseInt(string(r), 16, 64)
		if err != nil {
			n = 1
		}
		if n == 0 {
			t += 4
		} else {
			t += countZeroes(r)
			break
		}
	}
	return t
}
func countZeroes(n byte) int {
	binary := strconv.FormatUint(uint64(n), 2)
	numZeroes := strings.Count(binary, "0")
	return numZeroes - (len(binary) - 2) + 1
}
func countLeadingZeros(n int64) int {
	count := 0
	for i := 63; i >= 0; i-- {
		if (n & (1 << i)) == 0 {
			count++
		} else {
			break
		}
	}
	return count
}

func SHA256(data string) string {
	hasher := sha256.New()
	hasher.Write([]byte(data))
	return hex.EncodeToString(hasher.Sum(nil))
}
func GetGorgon(input string) string {
	body, err := json.Marshal(map[string]interface{}{
		"input": input,
		"key":   PSK,
	})
	if err != nil {
		log.Println("GetGorgon error:", err)
		return ""
	}

	req, err := http.NewRequest("POST", XGORGON_URL, bytes.NewBuffer(body))
	if err != nil {
		log.Println("GetGorgon error:", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("GetGorgon error:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 500 {
		log.Println("GetGorgon error: 500")
		return ""
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("GetGorgon error:", err)
		return ""
	}
	// slog.Info("Gorgon response is %s", string(respBody))
	var result map[string]string
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		log.Println("GetGorgon error:", err)
		return ""
	}
	gorgon, ok := result["x-gorgon"]
	if ok {
		return gorgon
	}

	log.Println("GetGorgon error: x-gorgon not found")
	return ""
}
func GetProxies() []string {
	proxies := []string{}
	// us os to check proxy.txt is exist
	if _, err := os.Stat("proxies.txt"); os.IsNotExist(err) {
		return proxies
	}
	// Read file
	data, err := os.ReadFile("proxies.txt")
	if err != nil {
		log.Println("ReadFile error:", err)
		return proxies
	}
	// Split by line
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			proxies = append(proxies, line)
		}
	}
	return proxies
}
func GetPSKValue() (string, error) {
	jss, err := GetHomeJS()
	if err != nil {
		slog.Error("GetPSK", slog.String("err", err.Error()))
		return "", err
	}
	for _, js := range jss {
		psk, _ := GetPSK(js)
		if psk != "" {
			return psk, nil
		}
	}
	return "", fmt.Errorf("GetPSK No PSK Found")
}
func UpdatePSKKey() {
	t := time.Tick(1 * time.Minute)
	lastUpdateTime := time.Now()
	for {
		<-t
		psk, err := GetPSKValue()
		if err != nil {
			slog.Error("UpdatePSKKey", slog.String("err", err.Error()))
		}
		if psk != "" {
			slog.Info("Get New PSK", slog.String("psk", psk))
			if psk != PSK {
				// Log Update Time duration
				slog.Info("🐲 Update PSK Time", slog.String("duration", time.Since(lastUpdateTime).String()))
				lastUpdateTime = time.Now()
				slog.Info("Update PSK", slog.String("psk", psk))
				PSK = psk
			}
		}
	}
}

type Messages struct {
	Msgtype string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

func SendMsg(apiUrl, msg string) {
	// json
	contentType := "application/json"
	sendData := Messages{
		Msgtype: "text",
		Text: struct {
			Content string `json:"content"`
		}{
			Content: msg,
		},
	}
	// request
	body, _ := json.Marshal(sendData)
	result, err := http.Post(apiUrl, contentType, strings.NewReader(string(body)))
	if err != nil {
		fmt.Printf("post failed, err:%v\n", err)
		return
	}
	defer result.Body.Close()
}

// sendLarkMessage sends a message to the specified Lark hook
func SendLarkMessage(url string, text string) error {
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
	resp, err := http.Post(url, contentType, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Add additional response handling here if needed

	return nil
}
