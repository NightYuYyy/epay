package demo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyReceiverRecordsAndFiltersCallbacks(t *testing.T) {
	store := NewNotifyStore(8)
	r := NewRouter(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/demo/notify?out_trade_no=ORDER-1&trade_no=TRADE-1&money=1.00&trade_status=TRADE_SUCCESS&sign=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "success" {
		t.Fatalf("notify response = status %d body %q, want 200 success", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/demo/notify-events?out_trade_no=ORDER-1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("events status = %d", w.Code)
	}

	var got struct {
		Code int `json:"code"`
		Data struct {
			Events []NotifyEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if got.Code != 0 || len(got.Data.Events) != 1 {
		t.Fatalf("events response = %#v", got)
	}
	event := got.Data.Events[0]
	if event.OutTradeNo != "ORDER-1" || event.TradeNo != "TRADE-1" || event.Params["trade_status"] != "TRADE_SUCCESS" {
		t.Fatalf("event = %#v", event)
	}
}

func TestNotifyReceiverRejectsUnsupportedMethods(t *testing.T) {
	r := NewRouter(NewNotifyStore(8))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/demo/notify", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
