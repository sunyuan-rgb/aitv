// Package web 提供 Gin 路由、会话鉴权与最小业务页面。
package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"aitv/internal/billing"
	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/refund"
	"aitv/internal/sales"
	"aitv/internal/store"
)

// SessionManager 内存会话（重启后需重新登录，业务数据不受影响，见假设A10）。
type SessionManager struct {
	mu   sync.Mutex
	data map[string]int64 // token -> user id
}

func NewSessions() *SessionManager {
	return &SessionManager{data: map[string]int64{}}
}

func (m *SessionManager) create(userID int64) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.data[token] = userID
	m.mu.Unlock()
	return token
}

func (m *SessionManager) drop(token string) {
	m.mu.Lock()
	delete(m.data, token)
	m.mu.Unlock()
}

func (m *SessionManager) userID(token string) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.data[token]
	return id, ok
}

// Server 聚合各业务服务。
type Server struct {
	St          *store.Store
	Sessions    *SessionManager
	Content     *content.Service
	Sales       *sales.Service
	Billing     *billing.Service
	Refund      *refund.Service
	TemplateDir string // 模板目录，默认 web/templates（测试时可覆盖）
}

// Router 构建 Gin 引擎。
func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	tplDir := s.TemplateDir
	if tplDir == "" {
		tplDir = "web/templates"
	}
	r.LoadHTMLGlob(tplDir + "/*.html")
	r.Static("/static", "./web/static")

	r.GET("/login", s.loginPage)
	r.POST("/login", s.loginAction)
	r.GET("/logout", s.logout)

	auth := r.Group("/", s.authRequired())
	auth.GET("/", s.home)

	// 编辑
	auth.GET("/editor", s.editorPage)
	auth.POST("/editor/submit", s.editorSubmit)
	// 审核
	auth.GET("/reviewer", s.reviewerPage)
	auth.POST("/reviewer/decision", s.reviewerDecision)
	// 运营
	auth.GET("/ops", s.opsPage)
	auth.POST("/ops/publish", s.opsPublish)
	auth.POST("/ops/revision/publish", s.opsPublishRevision)
	auth.POST("/ops/stop-sales", s.opsStopSales)
	auth.POST("/ops/pause", s.opsPause)
	auth.POST("/ops/resume", s.opsResume)
	// 用户
	auth.GET("/catalog", s.catalogPage)
	auth.GET("/play/:id", s.playPage)
	auth.POST("/buy", s.buyAction)
	auth.GET("/me", s.mePage)
	auth.POST("/refund", s.refundAction)
	// 客服
	auth.GET("/support", s.supportPage)
	auth.POST("/support/refund/decide", s.supportDecide)
	auth.GET("/support/explain", s.supportExplain)

	// JSON API（供并发测试与冒烟检查）
	api := auth.Group("/api")
	api.GET("/episodes", s.apiEpisodes)
	api.POST("/buy", s.apiBuy)
	api.GET("/watch/:id", s.apiWatch)
	api.GET("/me", s.apiMe)
	api.POST("/ops/action", s.apiOpsAction)
	api.POST("/content/submit", s.apiContentSubmit)
	api.POST("/review/decision", s.apiReviewDecision)

	return r
}

// ---------- 会话与用户 ----------

func (s *Server) currentUser(c *gin.Context) (model.User, bool) {
	token, err := c.Cookie("aitv_session")
	if err != nil || token == "" {
		return model.User{}, false
	}
	id, ok := s.Sessions.userID(token)
	if !ok {
		return model.User{}, false
	}
	row := s.St.DB.QueryRow(`SELECT id,username,role,balance FROM users WHERE id=?`, id)
	var u model.User
	if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.Balance); err != nil {
		return model.User{}, false
	}
	return u, true
}

func (s *Server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := s.currentUser(c)
		if !ok {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Set("user", u)
		c.Next()
	}
}

func userOf(c *gin.Context) model.User {
	v, _ := c.Get("user")
	return v.(model.User)
}

func (s *Server) loginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

func (s *Server) loginAction(c *gin.Context) {
	name := c.PostForm("username")
	pass := c.PostForm("password")
	var id int64
	var role string
	err := s.St.DB.QueryRow(`SELECT id,role FROM users WHERE username=? AND password=?`, name, pass).Scan(&id, &role)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "用户名或密码错误"})
		return
	}
	token := s.Sessions.create(id)
	c.SetCookie("aitv_session", token, 3600*8, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie("aitv_session"); err == nil {
		s.Sessions.drop(token)
	}
	c.SetCookie("aitv_session", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

func (s *Server) home(c *gin.Context) {
	u := userOf(c)
	switch u.Role {
	case model.RoleEditor:
		c.Redirect(http.StatusFound, "/editor")
	case model.RoleReviewer:
		c.Redirect(http.StatusFound, "/reviewer")
	case model.RoleOps:
		c.Redirect(http.StatusFound, "/ops")
	case model.RoleSupport:
		c.Redirect(http.StatusFound, "/support")
	default:
		c.Redirect(http.StatusFound, "/catalog")
	}
}

// ---------- 编辑 ----------

func (s *Server) editorPage(c *gin.Context) {
	u := userOf(c)
	if err := biz.CheckRole(u, model.RoleEditor); err != nil {
		c.String(http.StatusForbidden, "%s", err.Error())
		return
	}
	eps, err := s.Content.ListEpisodes()
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.HTML(http.StatusOK, "editor.html", gin.H{"User": u, "Episodes": eps, "Flash": c.Query("msg")})
}

func (s *Server) editorSubmit(c *gin.Context) {
	u := userOf(c)
	epID := parseID(c.PostForm("episode_id"))
	isFree := c.PostForm("is_free") == "1"
	rev, err := s.Content.SubmitForReview(u, epID, c.PostForm("title"), isFree, c.PostForm("play_url"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/editor"})
		return
	}
	msg := "已提交审核"
	if rev {
		msg = "修订已提交审核"
	}
	c.Redirect(http.StatusFound, "/editor?msg="+msg)
}

// ---------- 审核 ----------

func (s *Server) reviewerPage(c *gin.Context) {
	u := userOf(c)
	if err := biz.CheckRole(u, model.RoleReviewer); err != nil {
		c.String(http.StatusForbidden, "%s", err.Error())
		return
	}
	eps, err := s.Content.ListPendingReviews()
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	revs, _ := s.Content.ListPendingRevisions()
	c.HTML(http.StatusOK, "reviewer.html", gin.H{"User": u, "Episodes": eps, "Revisions": revs, "Flash": c.Query("msg")})
}

func (s *Server) reviewerDecision(c *gin.Context) {
	u := userOf(c)
	target := c.PostForm("target")
	id := parseID(c.PostForm("id"))
	approve := c.PostForm("decision") == "approve"
	err := s.Content.Review(u, target, id, approve, c.PostForm("reason"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/reviewer"})
		return
	}
	c.Redirect(http.StatusFound, "/reviewer?msg=处理成功")
}

// ---------- 运营 ----------

func (s *Server) opsPage(c *gin.Context) {
	u := userOf(c)
	if err := biz.CheckRole(u, model.RoleOps); err != nil {
		c.String(http.StatusForbidden, "%s", err.Error())
		return
	}
	sums, err := s.Sales.OpsSummary()
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	revs, _ := s.Content.ListApprovedRevisions()
	logs, _ := s.Sales.RecentLogs(20)
	c.HTML(http.StatusOK, "ops.html", gin.H{"User": u, "Summaries": sums, "Revisions": revs, "Logs": logs, "Flash": c.Query("msg")})
}

func (s *Server) opsPublish(c *gin.Context) {
	s.opsDo(c, func(u model.User) error {
		return s.Sales.Publish(u, parseID(c.PostForm("episode_id")))
	})
}

func (s *Server) opsPublishRevision(c *gin.Context) {
	s.opsDo(c, func(u model.User) error {
		return s.Sales.PublishRevision(u, parseID(c.PostForm("revision_id")))
	})
}

func (s *Server) opsStopSales(c *gin.Context) {
	s.opsDo(c, func(u model.User) error {
		return s.Sales.StopSales(u, parseID(c.PostForm("episode_id")), c.PostForm("reason"))
	})
}

func (s *Server) opsPause(c *gin.Context) {
	s.opsDo(c, func(u model.User) error {
		return s.Sales.Pause(u, parseID(c.PostForm("episode_id")), c.PostForm("reason"))
	})
}

func (s *Server) opsResume(c *gin.Context) {
	s.opsDo(c, func(u model.User) error {
		return s.Sales.Resume(u, parseID(c.PostForm("episode_id")), c.PostForm("reason"))
	})
}

func (s *Server) opsDo(c *gin.Context, fn func(model.User) error) {
	u := userOf(c)
	if err := fn(u); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/ops"})
		return
	}
	c.Redirect(http.StatusFound, "/ops?msg=操作成功")
}

// ---------- 用户 ----------

func (s *Server) catalogPage(c *gin.Context) {
	u := userOf(c)
	eps, err := s.Billing.GetPublished()
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.HTML(http.StatusOK, "catalog.html", gin.H{"User": u, "Episodes": eps, "Flash": c.Query("msg")})
}

func (s *Server) playPage(c *gin.Context) {
	u := userOf(c)
	epID := parseID(c.Param("id"))
	d, err := s.Billing.Watch(u, epID)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/catalog"})
		return
	}
	c.HTML(http.StatusOK, "play.html", gin.H{"User": u, "Decision": d, "EpisodeID": epID, "Flash": c.Query("msg")})
}

func (s *Server) buyAction(c *gin.Context) {
	s.buyDo(c, c.PostForm("request_id"))
}

func (s *Server) buyDo(c *gin.Context, requestID string) {
	u := userOf(c)
	epID := parseID(c.PostForm("episode_id"))
	_, _, err := s.Billing.Buy(u, epID, requestID)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/catalog"})
		return
	}
	c.Redirect(http.StatusFound, "/play/"+i642str(epID)+"?msg=购买成功")
}

func (s *Server) mePage(c *gin.Context) {
	u := userOf(c)
	rec, err := s.Billing.MyRecords(u)
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.HTML(http.StatusOK, "me.html", gin.H{"User": u, "Records": rec, "Flash": c.Query("msg")})
}

func (s *Server) refundAction(c *gin.Context) {
	u := userOf(c)
	epID := parseID(c.PostForm("episode_id"))
	_, err := s.Refund.Request(u, epID, c.PostForm("reason"))
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/me"})
		return
	}
	c.Redirect(http.StatusFound, "/me?msg=退款申请已提交，待客服处理")
}

// ---------- 客服 ----------

func (s *Server) supportPage(c *gin.Context) {
	u := userOf(c)
	if err := biz.CheckRole(u, model.RoleSupport); err != nil {
		c.String(http.StatusForbidden, "%s", err.Error())
		return
	}
	pending, err := s.Refund.PendingList()
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.HTML(http.StatusOK, "support.html", gin.H{"User": u, "Pending": pending, "Explain": nil, "Flash": c.Query("msg")})
}

func (s *Server) supportDecide(c *gin.Context) {
	u := userOf(c)
	id := parseID(c.PostForm("refund_id"))
	approve := c.PostForm("decision") == "approve"
	if err := s.Refund.Decide(u, id, approve, c.PostForm("reason")); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/support"})
		return
	}
	c.Redirect(http.StatusFound, "/support?msg=处理成功")
}

func (s *Server) supportExplain(c *gin.Context) {
	u := userOf(c)
	if err := biz.CheckRole(u, model.RoleSupport); err != nil {
		c.String(http.StatusForbidden, "%s", err.Error())
		return
	}
	ex, err := s.Refund.Explain(u, c.Query("username"), parseID(c.Query("episode_id")))
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"User": u, "Error": err.Error(), "Back": "/support"})
		return
	}
	pending, _ := s.Refund.PendingList()
	c.HTML(http.StatusOK, "support.html", gin.H{"User": u, "Pending": pending, "Explain": ex, "Flash": ""})
}

// ---------- JSON API ----------

// episodeDTO 用户可见剧集视图（不暴露草稿/工作站字段，符合"仅可浏览已上架内容"）。
type episodeDTO struct {
	ID      int64  `json:"id"`
	Seq     int    `json:"seq"`
	Title   string `json:"title"`
	IsFree  bool   `json:"is_free"`
	Price   int64  `json:"price"`
	PlayURL string `json:"play_url"`
	Status  string `json:"status"`
}

func (s *Server) apiEpisodes(c *gin.Context) {
	eps, err := s.Billing.GetPublished()
	if err != nil {
		jsonErr(c, err)
		return
	}
	out := make([]episodeDTO, 0, len(eps))
	for _, e := range eps {
		out = append(out, episodeDTO{
			ID: e.ID, Seq: e.Seq, Title: e.PubTitle, IsFree: e.PubIsFree,
			Price: e.PubPrice, PlayURL: e.PubPlayURL, Status: e.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{"episodes": out})
}

func (s *Server) apiBuy(c *gin.Context) {
	u := userOf(c)
	var req struct {
		EpisodeID int64  `json:"episode_id"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": biz.ErrBadRequest.Error()})
		return
	}
	p, e, err := s.Billing.Buy(u, req.EpisodeID, req.RequestID)
	if err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"purchase": p, "entitlement": e, "balance": s.balanceOf(u.ID)})
}

func (s *Server) apiWatch(c *gin.Context) {
	u := userOf(c)
	d, err := s.Billing.Watch(u, parseID(c.Param("id")))
	if err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"decision": d})
}

func (s *Server) apiMe(c *gin.Context) {
	u := userOf(c)
	rec, err := s.Billing.MyRecords(u)
	if err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (s *Server) apiOpsAction(c *gin.Context) {
	u := userOf(c)
	var req struct {
		Action    string `json:"action"` // publish|stop_sales|pause|resume
		EpisodeID int64  `json:"episode_id"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": biz.ErrBadRequest.Error()})
		return
	}
	var err error
	switch req.Action {
	case "publish":
		err = s.Sales.Publish(u, req.EpisodeID)
	case "stop_sales":
		err = s.Sales.StopSales(u, req.EpisodeID, req.Reason)
	case "pause":
		err = s.Sales.Pause(u, req.EpisodeID, req.Reason)
	case "resume":
		err = s.Sales.Resume(u, req.EpisodeID, req.Reason)
	default:
		err = biz.ErrBadRequest
	}
	if err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) apiContentSubmit(c *gin.Context) {
	u := userOf(c)
	var req struct {
		EpisodeID int64  `json:"episode_id"`
		Title     string `json:"title"`
		PlayURL   string `json:"play_url"`
		IsFree    bool   `json:"is_free"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": biz.ErrBadRequest.Error()})
		return
	}
	rev, err := s.Content.SubmitForReview(u, req.EpisodeID, req.Title, req.IsFree, req.PlayURL)
	if err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "is_revision": rev})
}

func (s *Server) apiReviewDecision(c *gin.Context) {
	u := userOf(c)
	var req struct {
		Target  string `json:"target"` // episode|revision
		ID      int64  `json:"id"`
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": biz.ErrBadRequest.Error()})
		return
	}
	if err := s.Content.Review(u, req.Target, req.ID, req.Approve, req.Reason); err != nil {
		jsonErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 工具 ----------

func (s *Server) balanceOf(userID int64) int64 {
	var bal int64
	_ = s.St.DB.QueryRow(`SELECT balance FROM users WHERE id=?`, userID).Scan(&bal)
	return bal
}

func jsonErr(c *gin.Context, err error) {
	code := http.StatusBadRequest
	if b, ok := biz.AsBiz(err); ok {
		if b.Code == biz.CodeForbidden {
			code = http.StatusForbidden
		}
		if b.Code == biz.CodeNotFound {
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{"error": b.Msg, "code": b.Code})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func parseID(s string) int64 {
	var v int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + int64(ch-'0')
	}
	return v
}

func i642str(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
