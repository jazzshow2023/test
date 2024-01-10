package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/scalalang2/golang-fifo/sieve"
)

var (
	URLS = []string{
		"wss://report-worker-ng.noscription.org",
		"wss://report-worker-2.noscription.org/",
	}
	ConnectCOUNT  = 10
	MessageChan   = make(chan []byte, 100)
	FIFO_CACHE    = sieve.New[string, bool](100)
	EVENT_ID_LOCK = sync.Mutex{}
	PublicHeaders = http.Header{
		"Pragma":          {"no-cache"},
		"Origin":          {"https://noscription.org"},
		"Accept-Language": {"en-US,en;q=0.9"},
		"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		"Cache-Control":   {"no-cache"},
	}
	Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
)

type ConnectionPool struct {
	Connections []*websocket.Conn
	mutex       sync.Mutex
}

func NewConnectionPool(size int) *ConnectionPool {
	// 初始化连接池的逻辑
	return &ConnectionPool{}
}

func (pool *ConnectionPool) AddConnection(conn *websocket.Conn) {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.Connections = append(pool.Connections, conn)
}

func (pool *ConnectionPool) BroadcastMessage(message []byte) {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	delectLock := sync.Mutex{}
	for idx, conn := range pool.Connections {
		go func(idx int, conn *websocket.Conn) {
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket error: %v", err)
				delectLock.Lock()
				// Fix  slice bounds out of range [2:1]
				if idx < len(pool.Connections) {
					pool.Connections = append(pool.Connections[:idx], pool.Connections[idx+1:]...)
				} else {
					pool.Connections = pool.Connections[:idx]
				}
				delectLock.Unlock()
				conn.Close()
			}
		}(idx, conn)
	}
}

func ConnectNoscripts(url string) {
	c, response, err := websocket.DefaultDialer.Dial(url, PublicHeaders)
	slog.Info(fmt.Sprintf("ConnectNoscripts url is %s", url))
	if err != nil {
		log.Println("Connect Noscripts Error dial:", err)
		if response != nil {
			log.Println("response status code :", response.StatusCode)
		}
		time.Sleep(time.Duration(Rand.Intn(10)) * time.Second)
		go ConnectNoscripts(url)
		return
	}
	defer c.Close()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("WS read Error:", err)
			time.Sleep(10 * time.Second)
			go ConnectNoscripts(url)
			return
		}
		EVENT_ID_LOCK.Lock()

		if FIFO_CACHE.Contains(string(message)) {
			EVENT_ID_LOCK.Unlock()
			continue
		} else {
			FIFO_CACHE.Set(string(message), true)
		}
		EVENT_ID_LOCK.Unlock()
		MessageChan <- message
	}
}
func StartConnectNoscripts() {
	for _, url := range URLS {
		for i := 0; i < ConnectCOUNT; i++ {
			go ConnectNoscripts(url)
			time.Sleep(10 * time.Second)
		}
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	pool := NewConnectionPool(10)

	r.GET("/", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println("Failed to upgrade to websocket:", err)
			return
		}
		pool.AddConnection(conn)
	})
	go StartConnectNoscripts()
	go func() {
		for {
			select {
			case message := <-MessageChan:
				slog.Info(fmt.Sprintf("MessageChan message is %s", string(message)))
				pool.BroadcastMessage(message)
			}
		}
	}()
	go func() {
		t := time.Tick(10 * time.Second)
		for {
			<-t
			slog.Info(fmt.Sprintf("Pool count is %d", len(pool.Connections)))
		}
	}()
	r.Run(":3001")
}
