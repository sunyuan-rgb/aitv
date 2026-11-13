package refund

import (
	"testing"

	"aitv/internal/biz"
	"aitv/internal/model"
)

func TestRequestRefundEpisodeNotFound(t *testing.T) {
	s, _, _, _, sv := newEnv(t)
	if _, err := sv.Request(U(s, "u1"), 999, "x"); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在剧集: %v", err)
	}
}

func TestRequestRefundAfterApproved(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	support := U(s, "support")
	pausePublished(t, s, c, sl, b, 3, u1)
	id, _ := sv.Request(u1, 3, "第一次")
	if err := sv.Decide(support, id, true, ""); err != nil {
		t.Fatal(err)
	}
	// 已批准后再申请：权益已撤销，不能再申请（只退一次由批准+权益撤销共同保证）
	if _, err := sv.Request(u1, 3, "第二次"); err == nil || bizCode(err) != biz.CodeBadState {
		t.Fatalf("批准后再申请: %v", err)
	}
	// 重复批准
	if err := sv.Decide(support, id, true, ""); err == nil {
		t.Fatal("重复批准应失败")
	}
}

func TestDecideNotFoundAndOpsExplain(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	// 不存在退款单
	if err := sv.Decide(U(s, "support"), 999, true, ""); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在退款单: %v", err)
	}
	// 运营也可核查（Explain 允许 ops）
	u1 := U(s, "u1")
	pausePublished(t, s, c, sl, b, 3, u1)
	_, _ = sv.Request(u1, 3, "要退款")
	ex, err := sv.Explain(U(s, "ops"), "u1", 3)
	if err != nil {
		t.Fatalf("ops explain: %v", err)
	}
	if ex.CanWatch {
		t.Fatal("暂停期间不可看")
	}
	var _ = model.RoleOps
}

func TestExplainRefundStatuses(t *testing.T) {
	s, c, sl, b, sv := newEnv(t)
	u1 := U(s, "u1")
	support := U(s, "support")
	pausePublished(t, s, c, sl, b, 3, u1)
	// 无退款记录时
	ex, _ := sv.Explain(support, "u1", 3)
	if ex.RefundStatus != "无记录" {
		t.Fatalf("退款状态=%s", ex.RefundStatus)
	}
	// 驳回后展示 rejected
	id, _ := sv.Request(u1, 3, "x")
	if err := sv.Decide(support, id, false, "不成立"); err != nil {
		t.Fatal(err)
	}
	ex, _ = sv.Explain(support, "u1", 3)
	if ex.RefundStatus != model.RefRejected {
		t.Fatalf("驳回后状态=%s", ex.RefundStatus)
	}
}

func TestPendingListEmpty(t *testing.T) {
	_, _, _, _, sv := newEnv(t)
	pending, err := sv.PendingList()
	if err != nil || pending != nil {
		t.Fatalf("空列表: %v %v", pending, err)
	}
}
