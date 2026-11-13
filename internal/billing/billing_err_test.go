package billing

import (
	"testing"
)

func TestMyRecordsWithRefunds(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	_, ent, _ := b.Buy(u1, 3, "r1")
	// 直接写入一条退款记录（退款工作流在 refund 包测试），覆盖用户退款查询分支
	_, err := s.DB.Exec(`INSERT INTO refunds(user_id,episode_id,entitlement_id,amount,reason,status,reject_reason,created_at)
		VALUES(?,?,?,?,?,?,?,datetime('now','localtime'))`,
		u1.ID, 3, ent.ID, 100, "资料有误", "approved", "", "2026-01-01 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := b.MyRecords(u1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Refunds) != 1 || rec.Refunds[0]["amount"].(int64) != 100 {
		t.Fatalf("退款记录: %+v", rec.Refunds)
	}
}

func TestErrBranchesOnClosedDB(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")
	_, _, _ = b.Buy(u1, 3, "r1")
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	// 关闭后所有查询走错误分支
	if _, _, err := b.Buy(u1, 3, "x"); err == nil {
		t.Fatal("关闭后 Buy 应失败")
	}
	if _, err := b.Watch(u1, 3); err == nil {
		t.Fatal("关闭后 Watch 应失败")
	}
	if _, err := b.MyRecords(u1); err == nil {
		t.Fatal("关闭后 MyRecords 应失败")
	}
	if _, err := b.GetPublished(); err == nil {
		t.Fatal("关闭后 GetPublished 应失败")
	}
	if _, _, _, err := b.findByRequestID("r1"); err == nil {
		t.Fatal("关闭后 findByRequestID 应失败")
	}
	if _, _, err := b.activeEntitlement(u1.ID, 3); err == nil {
		t.Fatal("关闭后 activeEntitlement 应失败")
	}
}
