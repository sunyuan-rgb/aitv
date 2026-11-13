package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestContentSubmitAPI(t *testing.T) {
	_, r := newServer(t)
	we := login(t, r, "editor")

	// 正常提交
	body := `{"episode_id":3,"title":"EP3","play_url":"/static/demo/ep3.mp4","is_free":false}`
	w := httptest2(r, httptestNewReq(http.MethodPost, "/api/content/submit", body, cookieOf(we), "application/json"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("提交审核失败: %d %s", w.Code, w.Body.String())
	}
	// 非法 JSON
	w = httptest2(r, httptestNewReq(http.MethodPost, "/api/content/submit", "bad", cookieOf(we), "application/json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法JSON应400: %d", w.Code)
	}
	// 非编辑角色
	wu := login(t, r, "u1")
	w = httptest2(r, httptestNewReq(http.MethodPost, "/api/content/submit", body, cookieOf(wu), "application/json"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("非编辑应403: %d", w.Code)
	}
	// 空标题
	w = httptest2(r, httptestNewReq(http.MethodPost, "/api/content/submit",
		`{"episode_id":3,"title":"","play_url":"x"}`, cookieOf(we), "application/json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空标题应400: %d", w.Code)
	}
}

func TestReviewDecisionAPI(t *testing.T) {
	_, r := newServer(t)
	we := login(t, r, "editor")
	_ = httptest2(r, httptestNewReq(http.MethodPost, "/api/content/submit",
		`{"episode_id":3,"title":"EP3","play_url":"/p.mp4"}`, cookieOf(we), "application/json"))

	wr := login(t, r, "reviewer")
	w := httptest2(r, httptestNewReq(http.MethodPost, "/api/review/decision",
		`{"target":"episode","id":3,"approve":true}`, cookieOf(wr), "application/json"))
	if w.Code != http.StatusOK {
		t.Fatalf("审核通过失败: %d %s", w.Code, w.Body.String())
	}
	// 重复审核 → 400
	w = httptest2(r, httptestNewReq(http.MethodPost, "/api/review/decision",
		`{"target":"episode","id":3,"approve":true}`, cookieOf(wr), "application/json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("重复审核应400: %d", w.Code)
	}
	// 非法 JSON
	w = httptest2(r, httptestNewReq(http.MethodPost, "/api/review/decision", "bad", cookieOf(wr), "application/json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法JSON应400: %d", w.Code)
	}
}
