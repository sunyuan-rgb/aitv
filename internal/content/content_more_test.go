package content

import (
	"strings"
	"testing"
)

func TestListEpisodes(t *testing.T) {
	_, sv := newTestEnv(t)
	eps, err := sv.ListEpisodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 6 {
		t.Fatalf("剧集数=%d", len(eps))
	}
	if eps[0].Seq != 1 || !eps[0].CurIsFree || eps[2].CurPrice != 100 {
		t.Fatalf("预置数据异常: %+v", eps[0])
	}
}

func TestSubmitForReviewPriceValidation(t *testing.T) {
	s, sv := newTestEnv(t)
	// 付费集价格为0时拒绝提交
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET cur_price=0 WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	_, err := sv.SubmitForReview(editor(s), 3, "t", false, "u")
	if err == nil || !strings.Contains(err.Error(), "价格") {
		t.Fatalf("价格为0应失败: %v", err)
	}
	// 免费集价格为0可提交
	if _, err := sv.SubmitForReview(editor(s), 1, "t", true, "u"); err != nil {
		t.Fatalf("免费集: %v", err)
	}
}

func TestSubmitPendingAgainAllowed(t *testing.T) {
	s, sv := newTestEnv(t)
	if _, err := sv.SubmitForReview(editor(s), 3, "v1", false, "u"); err != nil {
		t.Fatal(err)
	}
	// 待审状态再次提交（覆盖 pending_review 分支）
	if _, err := sv.SubmitForReview(editor(s), 3, "v2", false, "u2"); err != nil {
		t.Fatalf("待审再提交: %v", err)
	}
}

func TestReviewTargetValidation(t *testing.T) {
	s, sv := newTestEnv(t)
	// 非审核角色
	if err := sv.Review(editor(s), "episode", 1, true, ""); err == nil {
		t.Fatal("非审核角色应失败")
	}
	// 非法 target
	if err := sv.Review(reviewer(s), "bogus", 1, true, ""); err == nil {
		t.Fatal("非法target应失败")
	}
	// 不存在剧集
	if err := sv.Review(reviewer(s), "episode", 999, true, ""); err == nil {
		t.Fatal("不存在剧集应失败")
	}
	// 不存在修订
	if err := sv.Review(reviewer(s), "revision", 999, true, ""); err == nil {
		t.Fatal("不存在修订应失败")
	}
	// 非待审修订
	if _, err := sv.S.DB.Exec(`INSERT INTO revisions(episode_id,title,is_free,play_url,status,submitted_by)
		VALUES(1,'t',1,'u','rejected','editor')`); err != nil {
		t.Fatal(err)
	}
	if err := sv.Review(reviewer(s), "revision", 1, true, ""); err == nil {
		t.Fatal("非待审修订应失败")
	}
}
