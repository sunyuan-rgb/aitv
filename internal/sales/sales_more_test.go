package sales

import (
	"testing"

	"aitv/internal/billing"
	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/store"
)

func TestTransitionWrongStatesCoverStatusText(t *testing.T) {
	s, c, sv, _ := newEnv(t)
	ops := U(s, "ops")

	// draft 状态不可停止销售
	if err := sv.StopSales(ops, 1, "x"); err == nil {
		t.Fatal("草稿停售应失败")
	}
	// 提交审核（待审状态）
	if _, err := c.SubmitForReview(U(s, "editor"), 2, "t", true, "u"); err != nil {
		t.Fatal(err)
	}
	if err := sv.StopSales(ops, 2, "x"); err == nil {
		t.Fatal("待审停售应失败")
	}
	// 审核退回状态
	if _, err := c.SubmitForReview(U(s, "editor"), 3, "t", false, "u"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", 3, false, "原因"); err != nil {
		t.Fatal(err)
	}
	if err := sv.StopSales(ops, 3, "x"); err == nil {
		t.Fatal("退回态停售应失败")
	}
	// 审核通过未上架
	if _, err := c.SubmitForReview(U(s, "editor"), 4, "t", false, "u"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", 4, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := sv.Pause(ops, 4, "x"); err == nil {
		t.Fatal("approved 态暂停应失败")
	}
	// 上架后暂停→暂停态不能再停售
	publishEpisode(t, c, sv, s, 5)
	if err := sv.Pause(ops, 5, "x"); err != nil {
		t.Fatal(err)
	}
	if err := sv.StopSales(ops, 5, "x"); err == nil {
		t.Fatal("暂停态停售应失败")
	}
	// 不存在的剧集
	if err := sv.StopSales(ops, 999, "x"); err == nil || bizCode(err) != biz.CodeNotFound {
		t.Fatalf("不存在剧集: %v", err)
	}
	// 非运营
	if err := sv.StopSales(U(s, "editor"), 1, "x"); err == nil {
		t.Fatal("非运营应失败")
	}
	if err := sv.Resume(U(s, "editor"), 1, "x"); err == nil {
		t.Fatal("非运营 resume 应失败")
	}
}

func bizCode(err error) string {
	if b, ok := biz.AsBiz(err); ok {
		return b.Code
	}
	return ""
}

func TestPublishRevisionOnUnpublishedEpisode(t *testing.T) {
	s, _, sv, _ := newEnv(t)
	// 构造一条 approved 修订挂在草稿剧集上
	if _, err := s.DB.Exec(`INSERT INTO revisions(episode_id,title,is_free,play_url,status,submitted_by,reviewed_by)
		VALUES(1,'修订',1,'u','approved','editor','reviewer')`); err != nil {
		t.Fatal(err)
	}
	if err := sv.PublishRevision(U(s, "ops"), 1); err == nil {
		t.Fatal("未上架剧集发布修订应失败")
	}
}

func TestPublishOnMissingEpisode(t *testing.T) {
	st, _, sv, _ := newEnv(t)
	if err := sv.Publish(U(st, "ops"), 999); err == nil {
		t.Fatal("不存在剧集应失败")
	}
}

func TestOpsSummaryWithPausedAndRefund(t *testing.T) {
	st, c, sv, b := newEnvFull(t)
	// u1 购买第3集后暂停
	publishEpisode(t, c, sv, st, 3)
	_, _, _ = b.Buy(U(st, "u1"), 3, "r1")
	if err := sv.Pause(U(st, "ops"), 3, "暂停原因"); err != nil {
		t.Fatal(err)
	}
	sums, err := sv.OpsSummary()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range sums {
		if it.Episode.ID == 3 {
			if it.BuyerCount != 1 || it.ActiveHolders != 1 || it.LastAction != "暂停观看" || it.LastReason != "暂停原因" {
				t.Fatalf("汇总异常: %+v", it)
			}
		}
	}
	logs, _ := sv.RecentLogs(5)
	if len(logs) == 0 {
		t.Fatal("留痕为空")
	}
}

func newEnvFull(t *testing.T) (*store.Store, *content.Service, *Service, *billing.Service) {
	t.Helper()
	st, c, sv, b := newEnv(t)
	return st, c, sv, b
}
