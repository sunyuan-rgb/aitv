package billing

import (
	"testing"

	"aitv/internal/biz"
)

func TestMyRecordsFilled(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	_, _, _ = b.Buy(u1, 3, "r1")
	rec, err := b.MyRecords(u1)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Balance != 400 || len(rec.Purchases) != 1 || len(rec.Entitlements) != 1 {
		t.Fatalf("记录: %+v", rec)
	}
	if rec.Purchases[0]["title"] == "" {
		t.Fatal("购买记录应含剧集标题")
	}
}

func TestWatchDraftEpisode(t *testing.T) {
	s, _, _, b := newEnv(t)
	d, err := b.Watch(U(s, "u1"), 2) // 第2集草稿
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason == "" {
		t.Fatalf("草稿集: %+v", d)
	}
}

func TestBuyMissingPurchaseRow(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	_, _, _ = b.Buy(u1, 3, "r1")
	// 人为删除购买记录，模拟数据不一致（权益在、购买缺失）
	if _, err := s.DB.Exec(`DELETE FROM purchases WHERE request_id='r1'`); err != nil {
		t.Fatal(err)
	}
	_, _, err := b.Buy(u1, 3, "r1b")
	if err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("购买记录缺失应报错: %v", err)
	}
}

func TestGetPublishedScanBranches(t *testing.T) {
	s, _, _, b := newEnv(t)
	// 空目录
	eps, err := b.GetPublished()
	if err != nil || eps != nil {
		t.Fatalf("空目录: %v %v", eps, err)
	}
	_ = s
}

func TestWatchGetEpisodeErr(t *testing.T) {
	_, _, _, b := newEnv(t)
	u, _ := b.GetByUsername("u1")
	if _, err := b.Watch(u, 999); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("watch 不存在剧集: %v", err)
	}
}
