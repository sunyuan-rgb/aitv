package refund

import (
	"testing"

	"aitv/internal/biz"
)

func TestErrBranchesOnClosedDB(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)
	id, _ := sv.Request(u1, 3, "要退款")
	support := U(s, "support")
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	// 关闭后走错误分支
	if _, err := sv.Request(u1, 4, "x"); err == nil {
		t.Fatal("关闭后 Request 应失败")
	}
	if err := sv.Decide(support, id, true, ""); err == nil {
		t.Fatal("关闭后 Decide 应失败")
	}
	if _, err := sv.PendingList(); err == nil {
		t.Fatal("关闭后 PendingList 应失败")
	}
	if _, err := sv.Explain(support, "u1", 3); err == nil {
		t.Fatal("关闭后 Explain 应失败")
	}
}

func TestExplainEpisodeNotFound(t *testing.T) {
	s, _, _, _, sv := newEnv(t)
	if _, err := sv.Explain(U(s, "support"), "u1", 999); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在剧集: %v", err)
	}
}
