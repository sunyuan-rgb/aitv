package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// httptestNewReq 构造带 Cookie 与 Content-Type 的请求。
func httptestNewReq(method, path, body, cookie, contentType string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

// httptest2 执行请求并返回记录器。
func httptest2(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	return out
}

// newFormPost 构造表单 POST 请求。
func newFormPost(path string, form url.Values, cookie string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	return req
}

// postFormFail 断言表单提交失败（400 或 403）。
func postFormFail(t *testing.T, r *gin.Engine, path string, w *httptest.ResponseRecorder, form url.Values) {
	t.Helper()
	out := httptest2(r, newFormPost(path, form, cookieOf(w)))
	if out.Code != http.StatusBadRequest && out.Code != http.StatusForbidden {
		t.Fatalf("POST %s 应失败: %d %s", path, out.Code, out.Body.String())
	}
}

func TestHomeRedirectPerRole(t *testing.T) {
	_, r := newServer(t)
	for name, want := range map[string]string{
		"editor": "/editor", "reviewer": "/reviewer", "ops": "/ops", "support": "/support", "u1": "/catalog",
	} {
		w := login(t, r, name)
		wh := authGet(r, "/", w)
		if wh.Code != http.StatusFound || !strings.HasSuffix(wh.Header().Get("Location"), want) {
			t.Fatalf("%s 首页跳转=%d %s", name, wh.Code, wh.Header().Get("Location"))
		}
	}
}

func TestOpsPageActions(t *testing.T) {
	s, r := newServer(t)
	we := login(t, r, "editor")
	fullFlow(t, s, r, we)

	wOps := login(t, r, "ops")
	// 停止销售（页面表单）
	postForm(t, r, "/ops/stop-sales", wOps, url.Values{"episode_id": {"3"}, "reason": {"活动调整"}})
	// 恢复
	postForm(t, r, "/ops/resume", wOps, url.Values{"episode_id": {"3"}, "reason": {"活动恢复"}})
	// 暂停
	postForm(t, r, "/ops/pause", wOps, url.Values{"episode_id": {"3"}, "reason": {"资料有误"}})
	// 失败操作返回错误页
	form := url.Values{"episode_id": {"3"}, "reason": {"x"}}
	req := newFormPost("/ops/pause", form, cookieOf(wOps))
	out := httptest2(r, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("暂停态再暂停应400: %d", out.Code)
	}
	// 修订发布：先提交修订并审核通过
	postForm(t, r, "/editor/submit", we, url.Values{"episode_id": {"3"}, "title": {"改"}, "play_url": {"/n.mp4"}})
	wRev := login(t, r, "reviewer")
	// 找修订ID=1（首个修订）
	postForm(t, r, "/reviewer/decision", wRev, url.Values{"target": {"revision"}, "id": {"1"}, "decision": {"approve"}})
	postForm(t, r, "/ops/revision/publish", wOps, url.Values{"revision_id": {"1"}})
	// 运营页可渲染
	if w := authGet(r, "/ops", wOps); w.Code != http.StatusOK {
		t.Fatalf("ops page=%d", w.Code)
	}
}

func TestAPIBuyInvalidJSON(t *testing.T) {
	_, r := newServer(t)
	wu := login(t, r, "u1")
	req := httptestNewReq(http.MethodPost, "/api/buy", "not-json", cookieOf(wu), "application/json")
	out := httptest2(r, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("非法JSON=%d", out.Code)
	}
}

func TestAPIWatchNotFound(t *testing.T) {
	_, r := newServer(t)
	wu := login(t, r, "u1")
	out := httptest2(r, httptestNewReq(http.MethodGet, "/api/watch/999", "", cookieOf(wu), ""))
	if out.Code != http.StatusNotFound {
		t.Fatalf("watch 不存在=%d", out.Code)
	}
	out2 := httptest2(r, httptestNewReq(http.MethodGet, "/api/episodes", "", cookieOf(wu), ""))
	if out2.Code != http.StatusOK {
		t.Fatalf("episodes=%d", out2.Code)
	}
	out3 := httptest2(r, httptestNewReq(http.MethodGet, "/api/me", "", cookieOf(wu), ""))
	if out3.Code != http.StatusOK {
		t.Fatalf("me=%d", out3.Code)
	}
}

func TestSupportExplainErrors(t *testing.T) {
	_, r := newServer(t)
	ws := login(t, r, "support")
	// 不存在用户 → 400
	out := httptest2(r, httptestNewReq(http.MethodGet, "/support/explain?username=nobody&episode_id=1", "", cookieOf(ws), ""))
	if out.Code != http.StatusBadRequest {
		t.Fatalf("explain 不存在用户=%d", out.Code)
	}
}

func TestEditorAndReviewerPagesRender(t *testing.T) {
	_, r := newServer(t)
	we := login(t, r, "editor")
	if w := authGet(r, "/editor?msg=已提交审核", we); w.Code != http.StatusOK {
		t.Fatalf("editor page=%d", w.Code)
	}
	// 编辑访问编辑接口失败路径：非法表单
	form := url.Values{"episode_id": {"3"}, "title": {""}, "play_url": {""}}
	req := newFormPost("/editor/submit", form, cookieOf(we))
	out := httptest2(r, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("空标题=%d", out.Code)
	}
	wr := login(t, r, "reviewer")
	if w := authGet(r, "/reviewer", wr); w.Code != http.StatusOK {
		t.Fatalf("reviewer page=%d", w.Code)
	}
	// 退回无原因失败
	postFormFail(t, r, "/reviewer/decision", wr, url.Values{"target": {"episode"}, "id": {"3"}, "decision": {"reject"}, "reason": {""}})
	// 非审核角色决定失败
	postFormFail(t, r, "/reviewer/decision", we, url.Values{"target": {"episode"}, "id": {"3"}, "decision": {"approve"}})
}

func TestRefundFailuresViaPage(t *testing.T) {
	_, r := newServer(t)
	wu := login(t, r, "u1")
	// 未购买/非暂停集申请退款 → 400
	form := url.Values{"episode_id": {"3"}, "reason": {"x"}}
	req := newFormPost("/refund", form, cookieOf(wu))
	out := httptest2(r, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("退款失败页=%d", out.Code)
	}
	// 非用户角色（错误页展示无权限信息）
	ws := login(t, r, "support")
	form2 := url.Values{"episode_id": {"3"}, "reason": {"x"}}
	req2 := newFormPost("/refund", form2, cookieOf(ws))
	out2 := httptest2(r, req2)
	if out2.Code != http.StatusBadRequest || !strings.Contains(out2.Body.String(), "无权限") {
		t.Fatalf("客服申请退款=%d %s", out2.Code, out2.Body.String())
	}
}

func TestSupportDecideFailure(t *testing.T) {
	_, r := newServer(t)
	ws := login(t, r, "support")
	// 不存在的退款单
	form := url.Values{"refund_id": {"999"}, "decision": {"approve"}}
	req := newFormPost("/support/refund/decide", form, cookieOf(ws))
	out := httptest2(r, req)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("不存在退款单=%d", out.Code)
	}
}
