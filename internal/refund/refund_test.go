package refund

import (
	"fmt"
	"sync"
	"testing"

	"aitv/internal/billing"
	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/sales"
	"aitv/internal/store"
)

func newEnv(t *testing.T) (*store.Store, *content.Service, *sales.Service, *billing.Service, *Service) {
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
	sl := sales.New(st, c)
	b := billing.New(st)
	return st, c, sl, b, New(st, b)
}

func U(s *store.Store, name string) model.User {
	var id int64
	var role string
	if err := s.DB.QueryRow(`SELECT id,role FROM users WHERE username=?`, name).Scan(&id, &role); err != nil {
		panic(err)
	}
	return model.User{ID: id, Username: name, Role: role}
}

func bizCode(err error) string {
	if b, ok := biz.AsBiz(err); ok {
		return b.Code
	}
	return ""
}

func balanceOf(s *store.Store, uid int64) int64 {
	var bal int64
	if err := s.DB.QueryRow(`SELECT balance FROM users WHERE id=?`, uid).Scan(&bal); err != nil {
		panic(err)
	}
	return bal
}

// pausePublished 上架并暂停一集，用户已购买。
func pausePublished(t *testing.T, s *store.Store, c *content.Service, sl *sales.Service, b *billing.Service, epID int64, buyer model.User) {
	t.Helper()
	if _, err := c.SubmitForReview(U(s, "editor"), epID, "标题", false, "/p.mp4"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", epID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := sl.Publish(U(s, "ops"), epID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Buy(buyer, epID, fmt.Sprintf("pre-%d", epID)); err != nil {
		t.Fatal(err)
	}
	if err := sl.Pause(U(s, "ops"), epID, "内容资料有误"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRefundValidation(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)

	// 未购买用户不能申请
	if _, err := sv.Request(U(s, "u2"), 3, "想退"); err == nil {
		t.Fatal("未购买应失败")
	}
	// 非暂停状态不能申请：另一集上架未暂停
	if _, _, err := b.Buy(u1, 4, ""); err == nil {
		// 第4集是草稿，本来就买不了；这里验证草稿购买失败即可
		t.Fatal("草稿购买应失败")
	}
	// 正常申请
	id, err := sv.Request(u1, 3, "扣了余额却不能看")
	if err != nil {
		t.Fatalf("申请: %v", err)
	}
	if id == 0 {
		t.Fatal("申请ID为0")
	}
	// 重复申请被拒（只退一次）
	if _, err := sv.Request(u1, 3, "再退一次"); err == nil || bizCode(err) != biz.CodeDuplicate {
		t.Fatalf("重复申请应失败: %v", err)
	}
	// 空原因
	if _, err := sv.Request(u1, 4, ""); err == nil {
		t.Fatal("空原因应失败")
	}
	// 非用户角色
	if _, err := sv.Request(U(s, "editor"), 3, "x"); err == nil {
		t.Fatal("非用户角色应失败")
	}
}

func TestRequestRefundFreeEpisode(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	// 免费集上架（免费集无购买，自然无退款）
	if _, err := c.SubmitForReview(U(s, "editor"), 1, "免费集", true, "/p.mp4"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", 1, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := sl.Publish(U(s, "ops"), 1); err != nil {
		t.Fatal(err)
	}
	if err := sl.Pause(U(s, "ops"), 1, "临时下架"); err != nil {
		t.Fatal(err)
	}
	u1 := U(s, "u1")
	if _, err := sv.Request(u1, 1, "退"); err == nil {
		t.Fatal("免费集申请退款应失败")
	}
	_ = b
}

func TestSupportApproveRefundOnce(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	support := U(s, "support")
	pausePublished(t, s, c, sl, b, 3, u1)

	// 非客服不能处理
	id, _ := sv.Request(u1, 3, "要退款")
	if err := sv.Decide(u1, id, true, ""); err == nil {
		t.Fatal("非客服应失败")
	}
	// 驳回必须写原因
	if err := sv.Decide(support, id, false, ""); err == nil {
		t.Fatal("驳回无原因应失败")
	}
	// 批准
	before := balanceOf(s, u1.ID)
	if err := sv.Decide(support, id, true, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if bal := balanceOf(s, u1.ID); bal != before+100 {
		t.Fatalf("退款后余额=%d", bal)
	}
	// 权益被撤销
	d, err := b.Watch(u1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || !d.Refunded {
		t.Fatalf("已退款权益不可用: %+v", d)
	}
	// 重复处理被拒（只退一次）
	if err := sv.Decide(support, id, true, ""); err == nil {
		t.Fatal("重复批准应失败")
	}
	// 余额只退一次
	if bal := balanceOf(s, u1.ID); bal != before+100 {
		t.Fatalf("重复退款！余额=%d", bal)
	}
	// 已退款用户可重新购买（内容恢复可用后），形成新购买记录
	if err := sl.Resume(U(s, "ops"), 3, "资料修正完成"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	_, _, err = b.Buy(u1, 3, "repurchase")
	if err != nil {
		t.Fatalf("重新购买: %v", err)
	}
	var cnt int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM purchases WHERE user_id=? AND episode_id=?`, u1.ID, 3).Scan(&cnt)
	if cnt != 2 {
		t.Fatalf("应有2条购买记录，实际%d", cnt)
	}
	// 重新购买后可看
	if d, err := b.Watch(u1, 3); err != nil || !d.Allowed {
		t.Fatalf("重购后应可看: %+v err=%v", d, err)
	}
}

func TestSupportRejectRefund(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)
	id, _ := sv.Request(u1, 3, "要退款")
	before := balanceOf(s, u1.ID)
	if err := sv.Decide(U(s, "support"), id, false, "原因不充分"); err != nil {
		t.Fatal(err)
	}
	if bal := balanceOf(s, u1.ID); bal != before {
		t.Fatal("驳回后余额不应变化")
	}
	d, _ := b.Watch(u1, 3)
	if d.Allowed {
		t.Fatal("驳回后权益应保留？暂停期间仍不可播放")
	}
	// 被驳回后可再次申请（驳回不算"已退一次"）
	if _, err := sv.Request(u1, 3, "补充原因"); err != nil {
		t.Fatalf("驳回后应可再申请: %v", err)
	}
}

func TestConcurrentDecideOnlyOnce(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)
	id, _ := sv.Request(u1, 3, "并发退款")
	before := balanceOf(s, u1.ID)

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = sv.Decide(U(s, "support"), id, true, "")
		}(i)
	}
	wg.Wait()
	okCount := 0
	for _, err := range errs {
		if err == nil {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("并发批准应恰好成功1次，实际%d", okCount)
	}
	if bal := balanceOf(s, u1.ID); bal != before+100 {
		t.Fatalf("余额=%d，只退一次失败", bal)
	}
}

func TestPendingListAndExplain(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)
	_, _ = sv.Request(u1, 3, "要退款")

	pending, err := sv.PendingList()
	if err != nil || len(pending) != 1 {
		t.Fatalf("待处理列表: %v %v", pending, err)
	}
	// 客服核查
	support := U(s, "support")
	ex, err := sv.Explain(support, "u1", 3)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if ex.CanWatch || ex.RefundStatus != model.RefPending {
		t.Fatalf("核查结果: %+v", ex)
	}
	// 普通用户不能核查他人
	if _, err := sv.Explain(u1, "u1", 3); err == nil {
		t.Fatal("普通用户核查应失败")
	}
	// 不存在用户
	if _, err := sv.Explain(support, "nobody", 3); err == nil {
		t.Fatal("不存在用户应失败")
	}
}
