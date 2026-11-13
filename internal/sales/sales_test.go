package sales

import (
	"testing"

	"aitv/internal/billing"
	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/store"
)

func newEnv(t *testing.T) (*store.Store, *content.Service, *Service, *billing.Service) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	c := content.New(st)
	b := billing.New(st)
	return st, c, New(st, c), b
}

func U(s *store.Store, name string) model.User {
	var id int64
	var role string
	if err := s.DB.QueryRow(`SELECT id,role FROM users WHERE username=?`, name).Scan(&id, &role); err != nil {
		panic(err)
	}
	return model.User{ID: id, Username: name, Role: role}
}

// publishEpisode 走完整流程把一集上架，返回ID。
func publishEpisode(t *testing.T, c *content.Service, sv *Service, s *store.Store, epID int64) {
	t.Helper()
	if _, err := c.SubmitForReview(U(s, "editor"), epID, "标题", false, "/p.mp4"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", epID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := sv.Publish(U(s, "ops"), epID); err != nil {
		t.Fatal(err)
	}
}

func TestPublishRules(t *testing.T) {
	s, c, sv, _ := newEnv(t)

	// 非运营
	if err := sv.Publish(U(s, "editor"), 3); err == nil {
		t.Fatal("非运营应失败")
	}
	// 未审核通过不能上架
	if err := sv.Publish(U(s, "ops"), 3); err == nil {
		t.Fatal("草稿上架应失败")
	}
	// 正常上架
	publishEpisode(t, c, sv, s, 3)
	e, _ := sv.C.GetEpisode(3)
	if e.Status != model.EpPublished || e.PubTitle != "标题" || e.PubPrice != 100 {
		t.Fatalf("上架结果: %+v", e)
	}
	// 已上架再上架：状态机拒绝
	if err := sv.Publish(U(s, "ops"), 3); err == nil {
		t.Fatal("重复上架应失败")
	}
}

func TestStopSalesAndPause(t *testing.T) {
	s, c, sv, b := newEnv(t)
	publishEpisode(t, c, sv, s, 3)

	ops := U(s, "ops")
	u1 := U(s, "u1")

	// 原因必填
	if err := sv.StopSales(ops, 3, ""); err == nil {
		t.Fatal("停止销售无原因应失败")
	}
	// 购买前先买一集（老用户）
	if _, _, err := b.Buy(u1, 3, "req-1"); err != nil {
		t.Fatalf("buy: %v", err)
	}
	// 停止销售
	if err := sv.StopSales(ops, 3, "活动调整"); err != nil {
		t.Fatalf("stop sales: %v", err)
	}
	e, _ := sv.C.GetEpisode(3)
	if e.Status != model.EpSalesStopped {
		t.Fatalf("状态=%s", e.Status)
	}
	// 新用户购买被拒
	if _, _, err := b.Buy(U(s, "u2"), 3, "req-2"); err == nil {
		t.Fatal("停售后新购应失败")
	} else if b, ok := biz.AsBiz(err); !ok || b.Code != biz.CodeBadState {
		t.Fatalf("错误类型: %v", err)
	}
	// 老用户仍可观看
	d, err := b.Watch(u1, 3)
	if err != nil || !d.Allowed {
		t.Fatalf("老用户停售后应可看: %+v err=%v", d, err)
	}
	// 恢复销售
	if err := sv.Resume(ops, 3, "活动恢复"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, _, err := b.Buy(U(s, "u2"), 3, "req-3"); err != nil {
		t.Fatalf("恢复后新购应成功: %v", err)
	}
}

func TestPauseBlocksAll(t *testing.T) {
	s, c, sv, b := newEnv(t)
	publishEpisode(t, c, sv, s, 4)
	ops := U(s, "ops")
	u1 := U(s, "u1")
	if _, _, err := b.Buy(u1, 4, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Pause(ops, 4, "内容资料有误"); err != nil {
		t.Fatal(err)
	}
	// 购买被拒
	if _, _, err := b.Buy(U(s, "u2"), 4, "r2"); err == nil {
		t.Fatal("暂停期间购买应失败")
	}
	// 已购用户播放被拒
	d, err := b.Watch(u1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed {
		t.Fatal("暂停期间已购用户不应可看")
	}
	// 原因展示给用户
	if d.Reason == "" || !contains(d.Reason, "内容资料有误") {
		t.Fatalf("拒绝原因应包含运营原因: %s", d.Reason)
	}
	// 恢复后未退款用户恢复观看
	if err := sv.Resume(ops, 4, "资料修正"); err != nil {
		t.Fatal(err)
	}
	d, _ = b.Watch(u1, 4)
	if !d.Allowed {
		t.Fatal("恢复后应可看")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestResumeWrongState(t *testing.T) {
	s, c, sv, _ := newEnv(t)
	publishEpisode(t, c, sv, s, 3)
	// published 状态不能 resume
	if err := sv.Resume(U(s, "ops"), 3, "x"); err == nil {
		t.Fatal("published 状态 resume 应失败")
	}
}

func TestPublishRevision(t *testing.T) {
	s, c, sv, _ := newEnv(t)
	publishEpisode(t, c, sv, s, 3)

	// 提交修订并审核通过
	if _, err := c.SubmitForReview(U(s, "editor"), 3, "新标题", false, "/new.mp4"); err != nil {
		t.Fatal(err)
	}
	revs, _ := c.ListPendingRevisions()
	if err := c.Review(U(s, "reviewer"), "revision", revs[0]["id"].(int64), true, ""); err != nil {
		t.Fatal(err)
	}
	// 已通过修订待发布期间，可再提交新的待审修订（仅 pending 占位）
	if _, err := c.SubmitForReview(U(s, "editor"), 3, "再改", false, "/x.mp4"); err != nil {
		t.Fatalf("新修订提交: %v", err)
	}
	revs2, _ := c.ListPendingRevisions()
	if len(revs2) != 1 {
		t.Fatalf("期望1条待审，得到%d", len(revs2))
	}
	if err := c.Review(U(s, "reviewer"), "revision", revs2[0]["id"].(int64), false, "不改了"); err != nil {
		t.Fatal(err)
	}
	// 已退回的修订不能发布
	if err := sv.PublishRevision(U(s, "ops"), revs2[0]["id"].(int64)); err == nil {
		t.Fatal("退回修订不应可发布")
	}
	if err := sv.PublishRevision(U(s, "ops"), revs2[0]["id"].(int64)+1000); err == nil {
		t.Fatal("不存在修订应失败")
	}
	if err := sv.PublishRevision(U(s, "editor"), revs[0]["id"].(int64)); err == nil {
		t.Fatal("非运营应失败")
	}
	// 发布已通过的修订
	if err := sv.PublishRevision(U(s, "ops"), revs[0]["id"].(int64)); err != nil {
		t.Fatalf("publish revision: %v", err)
	}
	e, _ := sv.C.GetEpisode(3)
	if e.PubTitle != "新标题" || e.PubPlayURL != "/new.mp4" {
		t.Fatalf("修订未生效: %s %s", e.PubTitle, e.PubPlayURL)
	}
}

func TestOpsSummaryAndLogs(t *testing.T) {
	s, c, sv, b := newEnv(t)
	publishEpisode(t, c, sv, s, 3)
	_, _, _ = b.Buy(U(s, "u1"), 3, "r1")
	_, _, _ = b.Buy(U(s, "u1"), 3, "r1-dup") // 幂等不新增
	sums, err := sv.OpsSummary()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range sums {
		if it.Episode.ID == 3 {
			found = true
			if it.BuyerCount != 1 || it.ActiveHolders != 1 {
				t.Fatalf("购买人数=%d 有效权益=%d", it.BuyerCount, it.ActiveHolders)
			}
		}
	}
	if !found {
		t.Fatal("汇总缺少剧集3")
	}
	logs, err := sv.RecentLogs(10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("操作留痕: %v %v", logs, err)
	}
}
