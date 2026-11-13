package billing

import (
	"fmt"
	"sync"
	"testing"

	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/sales"
	"aitv/internal/store"
)

func newEnv(t *testing.T) (*store.Store, *content.Service, *sales.Service, *Service) {
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
	return st, c, sl, New(st)
}

func U(s *store.Store, name string) model.User {
	var id int64
	var role string
	if err := s.DB.QueryRow(`SELECT id,role FROM users WHERE username=?`, name).Scan(&id, &role); err != nil {
		panic(err)
	}
	return model.User{ID: id, Username: name, Role: role}
}

func publish(t *testing.T, s *store.Store, c *content.Service, sl *sales.Service, epID int64, free bool) {
	t.Helper()
	if _, err := c.SubmitForReview(U(s, "editor"), epID, "标题", free, "/p.mp4"); err != nil {
		t.Fatal(err)
	}
	if err := c.Review(U(s, "reviewer"), "episode", epID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := sl.Publish(U(s, "ops"), epID); err != nil {
		t.Fatal(err)
	}
}

func TestBuyHappyPath(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")

	p, e, err := b.Buy(u1, 3, "req-100")
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if p.Price != 100 || p.RequestID != "req-100" {
		t.Fatalf("购买记录: %+v", p)
	}
	if e.Status != model.EntActive {
		t.Fatalf("权益: %+v", e)
	}
	if bal := balanceOf(s, u1.ID); bal != 400 {
		t.Fatalf("余额=%d", bal)
	}
	// 观看成功
	d, err := b.Watch(u1, 3)
	if err != nil || !d.Allowed || d.PlayURL != "/p.mp4" {
		t.Fatalf("watch: %+v err=%v", d, err)
	}
	// 我的记录
	rec, err := b.MyRecords(u1)
	if err != nil || len(rec.Purchases) != 1 || len(rec.Entitlements) != 1 {
		t.Fatalf("records: %+v err=%v", rec, err)
	}
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

func TestBuyValidation(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	publish(t, s, c, sl, 4, false)
	publish(t, s, c, sl, 5, false)
	publish(t, s, c, sl, 6, false)
	publish(t, s, c, sl, 1, true) // 免费集
	u1 := U(s, "u1")

	// 免费集不可购买
	if _, _, err := b.Buy(u1, 1, ""); err == nil {
		t.Fatal("免费集购买应失败")
	}
	// 不存在的剧集
	if _, _, err := b.Buy(u1, 999, ""); err == nil {
		t.Fatal("不存在剧集应失败")
	}
	// 未上架剧集不可购买（第2集仍是草稿）
	if _, _, err := b.Buy(u1, 2, ""); err == nil {
		t.Fatal("未上架购买应失败")
	}
	// u2 余额300，买3集后第4次不足
	_, _, _ = b.Buy(U(s, "u2"), 3, "a1")
	_, _, _ = b.Buy(U(s, "u2"), 4, "a2")
	_, _, _ = b.Buy(U(s, "u2"), 5, "a3")
	_, _, err := b.Buy(U(s, "u2"), 6, "a4")
	if err == nil || bizCode(err) != biz.CodeNoBalance {
		t.Fatalf("余额不足应返回 insufficient_balance，得到 %v", err)
	}
	// 余额未扣
	if bal := balanceOf(s, U(s, "u2").ID); bal != 0 {
		t.Fatalf("余额=%d", bal)
	}
}

func TestBuyIdempotent(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")

	// 同 request_id 幂等
	p1, _, _ := b.Buy(u1, 3, "same")
	p2, _, err := b.Buy(u1, 3, "same")
	if err != nil {
		t.Fatalf("幂等请求应成功: %v", err)
	}
	if p1.ID != p2.ID {
		t.Fatal("幂等请求应返回同一购买记录")
	}
	if bal := balanceOf(s, u1.ID); bal != 400 {
		t.Fatalf("重复扣币！余额=%d", bal)
	}
	// 无 request_id 重复购买：已持权益不重复扣币
	_, _, err = b.Buy(u1, 3, "")
	if err != nil {
		t.Fatalf("重复购买应幂等成功: %v", err)
	}
	if bal := balanceOf(s, u1.ID); bal != 400 {
		t.Fatalf("再次扣币！余额=%d", bal)
	}
	// 同 request_id 不同参数：拒绝
	if _, _, err := b.Buy(u1, 4, "same"); err == nil || bizCode(err) != biz.CodeConflict {
		t.Fatalf("同编号不同参数应冲突: %v", err)
	}
}

func TestWatchPermissionMatrix(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 1, true)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")

	// 免费集直接可看
	if d, err := b.Watch(u1, 1); err != nil || !d.Allowed {
		t.Fatalf("免费集: %+v err=%v", d, err)
	}
	// 未购买付费集
	d, err := b.Watch(u1, 3)
	if err != nil || d.Allowed || d.Reason == "" {
		t.Fatalf("未购买: %+v err=%v", d, err)
	}
	// 购买后可看
	_, _, _ = b.Buy(u1, 3, "r1")
	if d, err := b.Watch(u1, 3); err != nil || !d.Allowed || !d.HasRight {
		t.Fatalf("已购: %+v err=%v", d, err)
	}
}

func TestConcurrentBuyOneAffordable(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	publish(t, s, c, sl, 4, false)
	u3 := U(s, "u3") // 余额100，只够一集

	const n = 2
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := b.Buy(u3, int64(3+i), fmt.Sprintf("conc-%d", i))
			results[i] = err
		}(i)
	}
	wg.Wait()
	okCount := 0
	for i, err := range results {
		if err == nil {
			okCount++
		} else if bizCode(err) != biz.CodeNoBalance {
			t.Fatalf("第%d个请求错误类型不对: %v", i, err)
		}
	}
	if okCount != 1 {
		t.Fatalf("并发买两集应恰好成功1次，实际%d", okCount)
	}
	if bal := balanceOf(s, u3.ID); bal != 0 {
		t.Fatalf("余额=%d，不应为负", bal)
	}
}

func TestConcurrentSameRequestId(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	u1 := U(s, "u1")

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = b.Buy(u1, 3, "one-req")
		}()
	}
	wg.Wait()
	var cnt int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM purchases WHERE user_id=? AND episode_id=?`, u1.ID, 3).Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("同一请求编号并发应只产生1条购买记录，实际%d", cnt)
	}
	if bal := balanceOf(s, u1.ID); bal != 400 {
		t.Fatalf("余额=%d", bal)
	}
}

func TestRaceDetectorConcurrentBuys(t *testing.T) {
	s, c, sl, b := newEnv(t)
	publish(t, s, c, sl, 3, false)
	publish(t, s, c, sl, 4, false)
	publish(t, s, c, sl, 5, false)
	publish(t, s, c, sl, 6, false)
	u1 := U(s, "u1") // 500币

	var wg sync.WaitGroup
	for i := int64(3); i <= 6; i++ {
		wg.Add(1)
		go func(ep int64) {
			defer wg.Done()
			_, _, _ = b.Buy(u1, ep, fmt.Sprintf("race-%d", ep))
		}(i)
	}
	wg.Wait()
	if bal := balanceOf(s, u1.ID); bal != 100 {
		t.Fatalf("4集×100币后余额应100，实际%d", bal)
	}
}
