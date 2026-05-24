package demo

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type NotifyEvent struct {
	ReceivedAt  time.Time         `json:"received_at"`
	OutTradeNo  string            `json:"out_trade_no"`
	TradeNo     string            `json:"trade_no"`
	Money       string            `json:"money"`
	TradeStatus string            `json:"trade_status"`
	Params      map[string]string `json:"params"`
}

type NotifyStore struct {
	mu     sync.RWMutex
	limit  int
	events []NotifyEvent
}

func NewNotifyStore(limit int) *NotifyStore {
	if limit <= 0 {
		limit = 100
	}
	return &NotifyStore{limit: limit}
}

func (s *NotifyStore) Add(event NotifyEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if extra := len(s.events) - s.limit; extra > 0 {
		copy(s.events, s.events[extra:])
		s.events = s.events[:s.limit]
	}
}

func (s *NotifyStore) List(outTradeNo string) []NotifyEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	outTradeNo = strings.TrimSpace(outTradeNo)
	out := make([]NotifyEvent, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		if outTradeNo == "" || event.OutTradeNo == outTradeNo {
			out = append(out, event)
		}
	}
	return out
}

func NewRouter(store *NotifyStore) *gin.Engine {
	r := gin.New()
	RegisterRoutes(r, store)
	return r
}

func RegisterRoutes(r gin.IRoutes, store *NotifyStore) {
	if store == nil {
		store = NewNotifyStore(100)
	}
	r.GET("/demo/notify", func(c *gin.Context) {
		params := make(map[string]string, len(c.Request.URL.Query()))
		keys := make([]string, 0, len(c.Request.URL.Query()))
		for key := range c.Request.URL.Query() {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			params[key] = c.Query(key)
		}
		store.Add(NotifyEvent{
			ReceivedAt:  time.Now().UTC(),
			OutTradeNo:  params["out_trade_no"],
			TradeNo:     params["trade_no"],
			Money:       params["money"],
			TradeStatus: params["trade_status"],
			Params:      params,
		})
		c.String(http.StatusOK, "success")
	})
	r.POST("/demo/notify", func(c *gin.Context) {
		c.String(http.StatusMethodNotAllowed, "method not allowed")
	})
	r.GET("/demo/notify-events", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "ok",
			"data": gin.H{"events": store.List(c.Query("out_trade_no"))},
		})
	})
}
