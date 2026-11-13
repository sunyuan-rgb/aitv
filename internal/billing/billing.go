// Package billing 实现购买扣币、单集权益与观看判定（题面规则3/4/5/7）。
package billing

import (
	"database/sql"
	"errors"
	"time"

	"aitv/internal/biz"
	"aitv/internal/model"
	"aitv/internal/store"
)

// Service 购买与权益服务。
type Service struct {
	S *store.Store
}

func New(s *store.Store) *Service { return &Service{S: s} }

// GetEpisode 读取剧集（公开字段为已发布版本）。
func (sv *Service) GetEpisode(id int64) (model.Episode, error) {
	row := sv.S.DB.QueryRow(`SELECT id,seq,status,pub_title,pub_is_free,pub_price,pub_play_url
		FROM episodes WHERE id=?`, id)
	var e model.Episode
	var pubFree int
	err := row.Scan(&e.ID, &e.Seq, &e.Status, &e.PubTitle, &pubFree, &e.PubPrice, &e.PubPlayURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, biz.ErrNotFound
		}
		return e, err
	}
	e.PubIsFree = pubFree == 1
	return e, nil
}

// GetByUsername 按用户名取账号。
func (sv *Service) GetByUsername(name string) (model.User, error) {
	row := sv.S.DB.QueryRow(`SELECT id,username,role,balance FROM users WHERE username=?`, name)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.Role, &u.Balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return u, biz.ErrNotFound
		}
		return u, err
	}
	return u, nil
}

// Buy 付费集购买（规则3/4）：
//   - 幂等：request_id 相同返回首次结果；同 request_id 不同请求参数拒绝；
//   - 已持有效权益重复购买：不重复扣币，返回既有权益；
//   - 扣币与权益写入同事务；余额条件更新保证并发不为负。
func (sv *Service) Buy(u model.User, epID int64, requestID string) (model.Purchase, model.Entitlement, error) {
	var purchase model.Purchase
	var ent model.Entitlement

	if requestID != "" {
		// 幂等检查：同编号复用
		p, e, ok, err := sv.findByRequestID(requestID)
		if err != nil {
			return purchase, ent, err
		}
		if ok {
			if p.UserID != u.ID || p.EpisodeID != epID {
				return purchase, ent, biz.NewBiz(biz.CodeConflict, "相同请求编号携带了不同请求参数，已拒绝")
			}
			return p, e, nil
		}
	}

	e, err := sv.GetEpisode(epID)
	if err != nil {
		return purchase, ent, err
	}
	switch e.Status {
	case model.EpPaused:
		return purchase, ent, biz.NewBiz(biz.CodeBadState, "该集暂停观看：暂停购买，原因为「"+pauseReason(sv, epID)+"」")
	case model.EpSalesStopped:
		return purchase, ent, biz.NewBiz(biz.CodeBadState, "该集已停止销售：仅已购用户可继续观看，原因为「"+stopReason(sv, epID)+"」")
	case model.EpPublished:
	default:
		return purchase, ent, biz.NewBiz(biz.CodeBadState, "该集未上架或不可购买")
	}
	if e.PubIsFree {
		return purchase, ent, biz.NewBiz(biz.CodeBadRequest, "免费集无需购买，直接观看")
	}

	// 已持有效权益：幂等返回，不重复扣币
	if entID, ok, err := sv.activeEntitlement(u.ID, epID); err != nil {
		return purchase, ent, err
	} else if ok {
		p, err2 := sv.lastPurchaseOf(u.ID, epID)
		if err2 != nil {
			return purchase, ent, err2
		}
		ent.ID = entID
		ent.Status = model.EntActive
		return p, ent, nil
	}

	tx, err := sv.S.DB.Begin()
	if err != nil {
		return purchase, ent, err
	}
	defer tx.Rollback()

	// 条件扣币：余额不足时影响行数为 0
	res, err := tx.Exec(`UPDATE users SET balance=balance-? WHERE id=? AND balance>=?`,
		e.PubPrice, u.ID, e.PubPrice)
	if err != nil {
		return purchase, ent, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return purchase, ent, biz.NewBiz(biz.CodeNoBalance, "平台币余额不足")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	rID := requestID
	if rID == "" {
		rID = genRequestID(u.ID, epID)
	}
	r, err := tx.Exec(`INSERT INTO purchases(request_id,user_id,episode_id,price,created_at) VALUES(?,?,?,?,?)`,
		rID, u.ID, epID, e.PubPrice, now)
	if err != nil {
		return purchase, ent, err
	}
	purchaseID, _ := r.LastInsertId()
	re, err := tx.Exec(`INSERT INTO entitlements(user_id,episode_id,purchase_id,status,created_at) VALUES(?,?,?,?,?)`,
		u.ID, epID, purchaseID, model.EntActive, now)
	if err != nil {
		return purchase, ent, err
	}
	entID, _ := re.LastInsertId()
	if err := tx.Commit(); err != nil {
		return purchase, ent, err
	}
	purchase = model.Purchase{ID: purchaseID, RequestID: rID, UserID: u.ID, EpisodeID: epID, Price: e.PubPrice, CreatedAt: now}
	ent = model.Entitlement{ID: entID, UserID: u.ID, EpisodeID: epID, PurchaseID: purchaseID, Status: model.EntActive, CreatedAt: now}
	return purchase, ent, nil
}

// Watch 观看判定（规则3/5/7）：
//   - 未上架/暂停 → 拒绝；停止销售 → 已购可看；
//   - 免费集直接放行；付费集需有效权益；已退款权益不可用但可重新购买。
type WatchDecision struct {
	Allowed  bool   `json:"allowed"`
	PlayURL  string `json:"play_url"`
	Title    string `json:"title"`
	IsFree   bool   `json:"is_free"`
	HasRight bool   `json:"has_right"`
	Refunded bool   `json:"refunded"` // 曾购已退款
	EpStatus string `json:"ep_status"`
	Reason   string `json:"reason"` // 拒绝原因（含运营操作原因）
}

func (sv *Service) Watch(u model.User, epID int64) (WatchDecision, error) {
	e, err := sv.GetEpisode(epID)
	if err != nil {
		return WatchDecision{}, err
	}
	d := WatchDecision{EpStatus: e.Status, Title: e.PubTitle, IsFree: e.PubIsFree, PlayURL: e.PubPlayURL}
	switch e.Status {
	case model.EpPaused:
		d.Reason = "该集已暂停观看（运营原因：" + pauseReason(sv, epID) + "），暂停期间所有用户（含已购）均不可播放"
		if refunded, rerr := sv.hasRefundedEntitlement(u.ID, epID); rerr == nil && refunded {
			d.Refunded = true
			d.Reason += "；你的权益已随退款撤销，内容恢复后可重新购买"
		}
		return d, nil
	case model.EpPublished, model.EpSalesStopped:
	default:
		d.Reason = "该集尚未上架"
		return d, nil
	}
	if e.PubIsFree {
		d.Allowed = true
		return d, nil
	}
	entID, ok, err := sv.activeEntitlement(u.ID, epID)
	if err != nil {
		return d, err
	}
	if ok {
		d.Allowed = true
		d.HasRight = true
		_ = entID
		return d, nil
	}
	refunded, rerr := sv.hasRefundedEntitlement(u.ID, epID)
	if rerr != nil {
		return d, rerr
	}
	if refunded {
		d.Refunded = true
		d.Reason = "你对该集的权益已随退款撤销，不可再用旧权益观看；可重新购买"
		return d, nil
	}
	d.Reason = "未购买该付费集，无观看权限"
	return d, nil
}

// MyRecords 用户自己的购买、权益、退款与余额（规则8：只看自己的）。
type MyRecords struct {
	Balance      int64
	Purchases    []map[string]any
	Entitlements []map[string]any
	Refunds      []map[string]any
}

func (sv *Service) MyRecords(u model.User) (*MyRecords, error) {
	rec := &MyRecords{}
	if err := sv.S.DB.QueryRow(`SELECT balance FROM users WHERE id=?`, u.ID).Scan(&rec.Balance); err != nil {
		return nil, err
	}
	rows, err := sv.S.DB.Query(`SELECT p.id,p.episode_id,p.price,p.created_at,e.pub_title
		FROM purchases p JOIN episodes e ON e.id=p.episode_id WHERE p.user_id=? ORDER BY p.id DESC`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, epID, price int64
		var at, title string
		if err := rows.Scan(&id, &epID, &price, &at, &title); err != nil {
			return nil, err
		}
		rec.Purchases = append(rec.Purchases, map[string]any{
			"id": id, "episode_id": epID, "price": price, "created_at": at, "title": title,
		})
	}
	rows2, err := sv.S.DB.Query(`SELECT en.id,en.episode_id,en.status,en.created_at,e.pub_title
		FROM entitlements en JOIN episodes e ON e.id=en.episode_id WHERE en.user_id=? ORDER BY en.id DESC`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var id, epID int64
		var status, at, title string
		if err := rows2.Scan(&id, &epID, &status, &at, &title); err != nil {
			return nil, err
		}
		rec.Entitlements = append(rec.Entitlements, map[string]any{
			"id": id, "episode_id": epID, "status": status, "created_at": at, "title": title,
		})
	}
	rows3, err := sv.S.DB.Query(`SELECT rf.id,rf.episode_id,rf.amount,rf.status,rf.reject_reason,rf.created_at,e.pub_title
		FROM refunds rf JOIN episodes e ON e.id=rf.episode_id WHERE rf.user_id=? ORDER BY rf.id DESC`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var id, epID, amount int64
		var status, reject, at, title string
		if err := rows3.Scan(&id, &epID, &amount, &status, &reject, &at, &title); err != nil {
			return nil, err
		}
		rec.Refunds = append(rec.Refunds, map[string]any{
			"id": id, "episode_id": epID, "amount": amount, "status": status,
			"reject_reason": reject, "created_at": at, "title": title,
		})
	}
	return rec, nil
}

// GetPublished 已上架（含停售/暂停，供目录展示状态）剧集。
func (sv *Service) GetPublished() ([]model.Episode, error) {
	rows, err := sv.S.DB.Query(`SELECT id,seq,status,pub_title,pub_is_free,pub_price,pub_play_url
		FROM episodes WHERE status IN (?,?,?) ORDER BY seq`,
		model.EpPublished, model.EpSalesStopped, model.EpPaused)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Episode
	for rows.Next() {
		var e model.Episode
		var free int
		if err := rows.Scan(&e.ID, &e.Seq, &e.Status, &e.PubTitle, &free, &e.PubPrice, &e.PubPlayURL); err != nil {
			return nil, err
		}
		e.PubIsFree = free == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

func (sv *Service) activeEntitlement(userID, epID int64) (int64, bool, error) {
	var id int64
	err := sv.S.DB.QueryRow(`SELECT id FROM entitlements WHERE user_id=? AND episode_id=? AND status=? ORDER BY id DESC LIMIT 1`,
		userID, epID, model.EntActive).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

func (sv *Service) hasRefundedEntitlement(userID, epID int64) (bool, error) {
	var n int
	err := sv.S.DB.QueryRow(`SELECT COUNT(*) FROM entitlements WHERE user_id=? AND episode_id=? AND status=?`,
		userID, epID, model.EntRefunded).Scan(&n)
	return n > 0, err
}

func (sv *Service) lastPurchaseOf(userID, epID int64) (model.Purchase, error) {
	var p model.Purchase
	err := sv.S.DB.QueryRow(`SELECT id,COALESCE(request_id,''),user_id,episode_id,price,created_at
		FROM purchases WHERE user_id=? AND episode_id=? ORDER BY id DESC LIMIT 1`, userID, epID).
		Scan(&p.ID, &p.RequestID, &p.UserID, &p.EpisodeID, &p.Price, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, biz.ErrNotFound
		}
		return p, err
	}
	return p, nil
}

func (sv *Service) findByRequestID(requestID string) (model.Purchase, model.Entitlement, bool, error) {
	var p model.Purchase
	err := sv.S.DB.QueryRow(`SELECT id,request_id,user_id,episode_id,price,created_at FROM purchases WHERE request_id=?`,
		requestID).Scan(&p.ID, &p.RequestID, &p.UserID, &p.EpisodeID, &p.Price, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, model.Entitlement{}, false, nil
		}
		return p, model.Entitlement{}, false, err
	}
	var ent model.Entitlement
	err = sv.S.DB.QueryRow(`SELECT id,user_id,episode_id,purchase_id,status,created_at FROM entitlements
		WHERE purchase_id=? ORDER BY id DESC LIMIT 1`, p.ID).
		Scan(&ent.ID, &ent.UserID, &ent.EpisodeID, &ent.PurchaseID, &ent.Status, &ent.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, ent, false, err
	}
	return p, ent, true, nil
}

func (sv *Service) lastAction(epID int64, action string) string {
	var reason string
	_ = sv.S.DB.QueryRow(`SELECT reason FROM op_logs WHERE episode_id=? AND action=? ORDER BY id DESC LIMIT 1`,
		epID, action).Scan(&reason)
	return reason
}

func pauseReason(sv *Service, epID int64) string { return sv.lastAction(epID, "暂停观看") }
func stopReason(sv *Service, epID int64) string  { return sv.lastAction(epID, "停止销售") }

func genRequestID(userID, epID int64) string {
	return time.Now().Format("20060102150405") + "-" + int642str(userID) + "-" + int642str(epID)
}

func int642str(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
