package content

import (
	"testing"

	"aitv/internal/biz"
	"aitv/internal/model"
	"aitv/internal/store"
)

func newTestEnv(t *testing.T) (*store.Store, *Service) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return st, New(st)
}

func getUser(s *store.Store, name string) model.User {
	var id int64
	var role string
	if err := s.DB.QueryRow(`SELECT id,role FROM users WHERE username=?`, name).Scan(&id, &role); err != nil {
		panic(err)
	}
	return model.User{ID: id, Username: name, Role: role}
}

func editor(s *store.Store) model.User     { return getUser(s, "editor") }
func reviewer(s *store.Store) model.User   { return getUser(s, "reviewer") }
func opsUser(s *store.Store) model.User    { return getUser(s, "ops") }
func normalUser(s *store.Store) model.User { return getUser(s, "u1") }

// submit+approve 一集，返回剧集ID。
func approvedEpisode(t *testing.T, s *store.Store, sv *Service, epID int64) int64 {
	t.Helper()
	if _, err := sv.SubmitForReview(editor(s), epID, "标题改", false, "/static/demo/x.mp4"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := sv.Review(reviewer(s), "episode", epID, true, ""); err != nil {
		t.Fatalf("review: %v", err)
	}
	return epID
}

// publishEpisodeDirect 直接置为已上架（避免测试包引入 sales 造成导入环）。
func publishEpisodeDirect(t *testing.T, s *store.Store, epID int64) {
	t.Helper()
	_, err := s.DB.Exec(`UPDATE episodes SET status='published',
		pub_title=cur_title, pub_is_free=cur_is_free, pub_price=cur_price, pub_play_url=cur_play_url
		WHERE id=?`, epID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmitForReviewRoleAndValidation(t *testing.T) {
	s, sv := newTestEnv(t)

	// 非编辑角色被拒绝
	if _, err := sv.SubmitForReview(reviewer(s), 1, "t", false, "u"); err == nil || bizCode(err) != biz.CodeForbidden {
		t.Fatalf("期望 forbidden，得到 %v", err)
	}
	// 必填字段
	if _, err := sv.SubmitForReview(editor(s), 1, "", false, "u"); err == nil {
		t.Fatal("空标题应失败")
	}
	// 不存在的剧集
	if _, err := sv.SubmitForReview(editor(s), 999, "t", false, "u"); err == nil {
		t.Fatal("不存在剧集应失败")
	}
}

func TestSubmitAndReviewFlow(t *testing.T) {
	s, sv := newTestEnv(t)

	// 提交审核
	rev, err := sv.SubmitForReview(editor(s), 3, "《霸总爱上我》第3集-修", false, "/static/demo/ep3.mp4")
	if err != nil || rev {
		t.Fatalf("submit: rev=%v err=%v", rev, err)
	}
	e, _ := sv.GetEpisode(3)
	if e.Status != model.EpPendingReview || e.SubmittedBy != "editor" {
		t.Fatalf("状态=%s 提交人=%s", e.Status, e.SubmittedBy)
	}

	// 审核退回：原因必填
	if err := sv.Review(reviewer(s), "episode", 3, false, ""); err == nil {
		t.Fatal("退回无原因应失败")
	}
	if err := sv.Review(reviewer(s), "episode", 3, false, "资料有误"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	e, _ = sv.GetEpisode(3)
	if e.Status != model.EpRejected || e.RejectReason != "资料有误" {
		t.Fatalf("退回状态=%s 原因=%s", e.Status, e.RejectReason)
	}

	// 退回后修改重新提交，再通过
	if _, err := sv.SubmitForReview(editor(s), 3, "《霸总爱上我》第3集-修2", false, "/static/demo/ep3v2.mp4"); err != nil {
		t.Fatalf("resubmit: %v", err)
	}
	if err := sv.Review(reviewer(s), "episode", 3, true, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	e, _ = sv.GetEpisode(3)
	if e.Status != model.EpApproved {
		t.Fatalf("状态=%s", e.Status)
	}

	// 重复审核已通过内容：状态不对
	if err := sv.Review(reviewer(s), "episode", 3, true, ""); err == nil {
		t.Fatal("重复审核应失败")
	}
}

func TestSelfReviewForbidden(t *testing.T) {
	s, sv := newTestEnv(t)
	// 编辑用 reviewer 同名提交（模拟自审）
	if _, err := sv.S.DB.Exec(`INSERT INTO users(username,password,role,balance) VALUES('self','x','editor',0)`); err != nil {
		t.Fatal(err)
	}
	self := getUser(s, "self")
	if _, err := sv.SubmitForReview(self, 1, "t", true, "u"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// 同名审核人员
	if _, err := sv.S.DB.Exec(`INSERT INTO users(username,password,role,balance) VALUES('self2','x','reviewer',0)`); err != nil {
		t.Fatal(err)
	}
	// 把提交人改成同名审核员
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET submitted_by='r1' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := sv.S.DB.Exec(`INSERT INTO users(username,password,role,balance) VALUES('r1','x','reviewer',0)`); err != nil {
		t.Fatal(err)
	}
	r1 := getUser(s, "r1")
	if err := sv.Review(r1, "episode", 1, true, ""); err == nil || bizCode(err) != biz.CodeForbidden {
		t.Fatalf("自审应被拒绝，得到 %v", err)
	}
}

// bizCode 取业务错误码（无业务错误返回空串）。
func bizCode(err error) string {
	if b, ok := biz.AsBiz(err); ok {
		return b.Code
	}
	return ""
}

func TestRevisionFlow(t *testing.T) {
	s, sv := newTestEnv(t)
	approvedEpisode(t, s, sv, 3)

	// 未上架内容修改走重新审核，不产生修订
	rev, err := sv.SubmitForReview(editor(s), 3, "改标题", false, "/x.mp4")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if rev {
		t.Fatal("未上架提交不应产生修订")
	}
	// 审核+上架
	if err := sv.Review(reviewer(s), "episode", 3, true, ""); err != nil {
		t.Fatal(err)
	}
	publishEpisodeDirect(t, s, 3)

	// 已上架：修改免费/付费属性被拒绝
	if _, err := sv.SubmitForReview(editor(s), 3, "改标题", true, "/x.mp4"); err == nil {
		t.Fatal("已上架改免费属性应失败")
	}
	// 已上架：正常修订
	rev, err = sv.SubmitForReview(editor(s), 3, "改标题2", false, "/x2.mp4")
	if err != nil || !rev {
		t.Fatalf("revision: rev=%v err=%v", rev, err)
	}
	// 第二份待审修订被拒
	if _, err := sv.SubmitForReview(editor(s), 3, "改标题3", false, "/x3.mp4"); err == nil {
		t.Fatal("第二份待审修订应失败")
	}
	revs, err := sv.ListPendingRevisions()
	if err != nil || len(revs) != 1 {
		t.Fatalf("pending revisions: %v %v", revs, err)
	}
	revID := revs[0]["id"].(int64)

	// 自审修订被拒
	if err := sv.Review(editor(s), "revision", revID, true, ""); err == nil {
		t.Fatal("角色不对应失败")
	}
	if err := sv.Review(reviewer(s), "revision", revID, false, "播放地址有误"); err != nil {
		t.Fatalf("reject revision: %v", err)
	}
	// 退回后可重新提交
	if _, err := sv.SubmitForReview(editor(s), 3, "改标题4", false, "/x4.mp4"); err != nil {
		t.Fatalf("resubmit revision: %v", err)
	}
	revs, _ = sv.ListPendingRevisions()
	revID = revs[0]["id"].(int64)
	if err := sv.Review(reviewer(s), "revision", revID, true, ""); err != nil {
		t.Fatal(err)
	}
	appRevs, err := sv.ListApprovedRevisions()
	if err != nil || len(appRevs) != 1 {
		t.Fatalf("approved revisions: %v %v", appRevs, err)
	}
	_ = appRevs
}

func TestListPendingReviews(t *testing.T) {
	s, sv := newTestEnv(t)
	// 提交两集
	if _, err := sv.SubmitForReview(editor(s), 1, "t1", true, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := sv.SubmitForReview(editor(s), 2, "t2", true, "u2"); err != nil {
		t.Fatal(err)
	}
	pending, err := sv.ListPendingReviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("期望2条待审，得到 %d", len(pending))
	}
}
