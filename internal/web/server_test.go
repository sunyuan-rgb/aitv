package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"aitv/internal/billing"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/refund"
	"aitv/internal/sales"
	"aitv/internal/store"
)

func newServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	c := content.New(st)
	s := &Server{
		St:          st,
		Sessions:    NewSessions(),
		Content:     c,
		Sales:       sales.New(st, c),
		Billing:     billing.New(st),
		Refund:      refund.New(st, billing.New(st)),
		TemplateDir: "../../web/templates",
	}
	return s, s.Router()
}

// login 登录拿会话 cookie。
func login(t *testing.T, r *gin.Engine, name string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {name}, "password": {"pass123"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authGet(r *gin.Engine, path string, w *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Cookie", cookieOf(w))
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	return out
}

func cookieOf(w *httptest.ResponseRecorder) string {
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "aitv_session" {
			return ck.Name + "=" + ck.Value
		}
	}
	return ""
}

func TestLoginFlow(t *testing.T) {
	_, r := newServer(t)

	// 登录页
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("登录页=%d", w.Code)
	}
	// 密码错误
	form := url.Values{"username": {"u1"}, "password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码=%d", w2.Code)
	}
	// 未登录访问受保护页面重定向
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	if w3.Code != http.StatusFound {
		t.Fatalf("未登录应302，得到%d", w3.Code)
	}
	// 登录成功
	w4 := login(t, r, "u1")
	if w4.Code != http.StatusFound {
		t.Fatalf("登录=%d", w4.Code)
	}
	if cookieOf(w4) == "" {
		t.Fatal("未种下会话cookie")
	}
	// 登录后访问目录
	w5 := authGet(r, "/catalog", w4)
	if w5.Code != http.StatusOK {
		t.Fatalf("目录=%d", w5.Code)
	}
	// 退出
	w6 := authGet(r, "/logout", w4)
	if w6.Code != http.StatusFound {
		t.Fatalf("退出=%d", w6.Code)
	}
}

func TestRoleGuardsOnPages(t *testing.T) {
	_, r := newServer(t)
	// 普通用户访问运营页
	w := login(t, r, "u1")
	w2 := authGet(r, "/ops", w)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("用户访问运营页=%d", w2.Code)
	}
	// 编辑访问客服页
	we := login(t, r, "editor")
	w3 := authGet(r, "/support", we)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("编辑访问客服页=%d", w3.Code)
	}
	// 各角色工作台可达
	for name, path := range map[string]string{
		"editor": "/editor", "reviewer": "/reviewer", "ops": "/ops", "support": "/support",
	} {
		wl := login(t, r, name)
		wp := authGet(r, path, wl)
		if wp.Code != http.StatusOK {
			t.Fatalf("%s访问%s=%d", name, path, wp.Code)
		}
	}
}

// fullFlow 完整走一遍：提交审核→通过→上架→购买→观看。
func fullFlow(t *testing.T, s *Server, r *gin.Engine, wEditor *httptest.ResponseRecorder) {
	// 编辑提交第3集审核
	postForm(t, r, "/editor/submit", wEditor, url.Values{
		"episode_id": {"3"}, "title": {"第3集"}, "play_url": {"/static/demo/ep3.mp4"},
	})
	// 审核通过
	wRev := login(t, r, "reviewer")
	postForm(t, r, "/reviewer/decision", wRev, url.Values{
		"target": {"episode"}, "id": {"3"}, "decision": {"approve"},
	})
	// 运营上架
	wOps := login(t, r, "ops")
	postForm(t, r, "/ops/publish", wOps, url.Values{"episode_id": {"3"}})
}

func postForm(t *testing.T, r *gin.Engine, path string, w *httptest.ResponseRecorder, form url.Values) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookieOf(w))
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	if out.Code == http.StatusBadRequest || out.Code == http.StatusForbidden {
		t.Fatalf("POST %s 失败: %d %s", path, out.Code, out.Body.String())
	}
}

func TestFullUserFlowViaAPI(t *testing.T) {
	s, r := newServer(t)
	we := login(t, r, "editor")
	fullFlow(t, s, r, we)

	// 用户购买
	wu := login(t, r, "u1")
	body := `{"episode_id":3,"request_id":"api-req-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/buy", strings.NewReader(body))
	req.Header.Set("Cookie", cookieOf(wu))
	req.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("api buy=%d %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["balance"].(float64) != 400 {
		t.Fatalf("余额=%v", resp["balance"])
	}
	// 幂等重试
	req2 := httptest.NewRequest(http.MethodPost, "/api/buy", strings.NewReader(body))
	req2.Header.Set("Cookie", cookieOf(wu))
	req2.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req2)
	var resp2 map[string]any
	_ = json.Unmarshal(w3.Body.Bytes(), &resp2)
	if resp2["balance"].(float64) != 400 {
		t.Fatalf("幂等后余额=%v", resp2["balance"])
	}
	// 观看
	w4 := authGet(r, "/api/watch/3", wu)
	if w4.Code != http.StatusOK || !strings.Contains(w4.Body.String(), `"allowed":true`) {
		t.Fatalf("watch=%d %s", w4.Code, w4.Body.String())
	}
	// 目录与我的记录
	if w5 := authGet(r, "/api/episodes", wu); w5.Code != http.StatusOK {
		t.Fatalf("episodes=%d", w5.Code)
	}
	if w6 := authGet(r, "/api/me", wu); w6.Code != http.StatusOK {
		t.Fatalf("me=%d", w6.Code)
	}
	// 页面：观看页与我的记录页
	if wp := authGet(r, "/play/3", wu); wp.Code != http.StatusOK {
		t.Fatalf("play page=%d", wp.Code)
	}
	if wp := authGet(r, "/me", wu); wp.Code != http.StatusOK {
		t.Fatalf("me page=%d", wp.Code)
	}
}

func TestOpsStopSalesViaAPI(t *testing.T) {
	s, r := newServer(t)
	we := login(t, r, "editor")
	fullFlow(t, s, r, we)

	wOps := login(t, r, "ops")
	body := `{"action":"stop_sales","episode_id":3,"reason":"活动调整"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ops/action", strings.NewReader(body))
	req.Header.Set("Cookie", cookieOf(wOps))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop sales=%d %s", w.Code, w.Body.String())
	}
	// 非法动作
	body2 := `{"action":"bogus","episode_id":3}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/ops/action", strings.NewReader(body2))
	req2.Header.Set("Cookie", cookieOf(wOps))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("bogus action=%d", w2.Code)
	}
	// 新用户不能购买停售剧集
	wu2 := login(t, r, "u2")
	req3 := httptest.NewRequest(http.MethodPost, "/api/buy", strings.NewReader(`{"episode_id":3}`))
	req3.Header.Set("Cookie", cookieOf(wu2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("停售后购买=%d", w3.Code)
	}
	// 非运营调用 ops action
	wu := login(t, r, "u1")
	req4 := httptest.NewRequest(http.MethodPost, "/api/ops/action", strings.NewReader(body))
	req4.Header.Set("Cookie", cookieOf(wu))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusForbidden {
		t.Fatalf("非运营ops=%d", w4.Code)
	}
}

func TestSupportRefundViaPages(t *testing.T) {
	s, r := newServer(t)
	we := login(t, r, "editor")
	fullFlow(t, s, r, we)

	// 用户购买第3集并让运营暂停
	wu := login(t, r, "u1")
	postForm(t, r, "/buy", wu, url.Values{"episode_id": {"3"}, "request_id": {"r1"}})
	wOps := login(t, r, "ops")
	postForm(t, r, "/ops/pause", wOps, url.Values{"episode_id": {"3"}, "reason": {"内容资料有误"}})

	// 用户观看被拒（页面），申请退款
	wPlay := authGet(r, "/play/3", wu)
	if wPlay.Code != http.StatusOK || !strings.Contains(wPlay.Body.String(), "无法观看") {
		t.Fatalf("play页面应展示拒绝原因: %d", wPlay.Code)
	}
	postForm(t, r, "/refund", wu, url.Values{"episode_id": {"3"}, "reason": {"扣了余额不能看"}})

	// 客服批准
	ws := login(t, r, "support")
	postForm(t, r, "/support/refund/decide", ws, url.Values{"refund_id": {"1"}, "decision": {"approve"}})

	// 用户余额退回、权益撤销
	var bal int64
	_ = s.St.DB.QueryRow(`SELECT balance FROM users WHERE username='u1'`).Scan(&bal)
	if bal != 500 { // 500 - 100（购买） + 100（退款）= 500
		t.Fatalf("退款后余额=%d", bal)
	}
	// 客服核查接口
	wEx := authGet(r, "/support/explain?username=u1&episode_id=3", ws)
	if wEx.Code != http.StatusOK {
		t.Fatalf("explain=%d", wEx.Code)
	}
}

func TestParseID(t *testing.T) {
	if parseID("12") != 12 || parseID("") != 0 || parseID("ab") != 0 {
		t.Fatal("parseID 异常")
	}
	if i642str(123) != "123" || i642str(0) != "0" {
		t.Fatal("i642str 异常")
	}
}

var _ = model.RoleEditor // 保持 model 引用
