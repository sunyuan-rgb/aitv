package billing

import (
	"strings"
	"testing"

	"aitv/internal/biz"
)

func TestGetByUsername(t *testing.T) {
	_, _, _, b := newEnv(t)
	u, err := b.GetByUsername("u1")
	if err != nil || u.ID == 0 || u.Balance != 500 {
		t.Fatalf("u1: %+v err=%v", u, err)
	}
	if _, err := b.GetByUsername("nobody"); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在用户: %v", err)
	}
}

func TestGetPublished(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	publish(t, s, c, sl, 1, true)
	eps, err := b.GetPublished()
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("已上架数=%d", len(eps))
	}
}

func TestBuyWithoutRequestIDGeneratesOne(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	p, _, err := b.Buy(u1, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.RequestID, "-") {
		t.Fatalf("自动请求编号=%s", p.RequestID)
	}
}

func TestWatchShowsReasons(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	ops := U(s, "ops")
	u1 := U(s, "u1")

	// 停售后老用户观看：拒绝原因不含运营原因（可看），新用户购买失败原因含原因
	_, _, _ = b.Buy(u1, 3, "r1")
	_ = sl.StopSales(ops, 3, "活动结束")
	if _, _, err := b.Buy(U(s, "u2"), 3, "r2"); err == nil || !strings.Contains(err.Error(), "活动结束") {
		t.Fatalf("停售购买原因: %v", err)
	}
	// 恢复销售后暂停，再验证暂停原因
	_ = sl.Resume(ops, 3, "恢复")
	_ = sl.Pause(ops, 3, "资料有误")
	if _, _, err := b.Buy(u1, 3, "r3"); err == nil || !strings.Contains(err.Error(), "资料有误") {
		t.Fatalf("暂停购买原因: %v", err)
	}
	d, _ := b.Watch(u1, 3)
	if !strings.Contains(d.Reason, "资料有误") {
		t.Fatalf("暂停观看原因: %s", d.Reason)
	}
}

func TestWatchPausedRefundedUser(t *testing.T) {
	// 已退款用户在暂停期间的提示（配合 refund 包场景，用 SQL 直接模拟）
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	_, _, _ = b.Buy(u1, 3, "r1")
	// 直接把权益置为已退款并暂停
	_, _ = s.DB.Exec(`UPDATE entitlements SET status='refunded' WHERE user_id=?`, u1.ID)
	_ = sl.Pause(U(s, "ops"), 3, "暂停")
	d, err := b.Watch(u1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || !d.Refunded || !strings.Contains(d.Reason, "退款") {
		t.Fatalf("暂停+已退款: %+v", d)
	}
}

func TestGetEpisodeNotFoundAndBadInput(t *testing.T) {
	_, _, _, b := newEnv(t)
	if _, err := b.GetEpisode(999); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在剧集: %v", err)
	}
}

func TestMyRecordsEmpty(t *testing.T) {
	s, _, _, b := newEnv(t)
	rec, err := b.MyRecords(U(s, "u1"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Balance != 500 || rec.Purchases != nil || rec.Entitlements != nil || rec.Refunds != nil {
		t.Fatalf("空记录: %+v", rec)
	}
}
