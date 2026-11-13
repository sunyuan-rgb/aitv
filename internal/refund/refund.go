// Package refund 实现暂停观看期间的退款申请与客服处理（题面规则6/7/8）。
package refund

import (
	"database/sql"
	"errors"
	"time"

	"aitv/internal/billing"
	"aitv/internal/biz"
	"aitv/internal/model"
	"aitv/internal/store"
)

// Service 退款服务。
type Service struct {
	S *store.Store
	B *billing.Service
}

func New(s *store.Store, b *billing.Service) *Service { return &Service{S: s, B: b} }

// Request 用户对暂停观看的付费集申请退款（规则6：只退一次；免费集无退款；未购不可申请）。
func (sv *Service) Request(u model.User, epID int64, reason string) (int64, error) {
	if u.Role != model.RoleUser {
		return 0, biz.ErrForbidden
	}
	if err := biz.RequireNonEmpty(map[string]string{"退款原因": reason}); err != nil {
		return 0, err
	}
	e, err := sv.B.GetEpisode(epID)
	if err != nil {
		return 0, err
	}
	if e.PubIsFree {
		return 0, biz.NewBiz(biz.CodeBadRequest, "免费集不涉及退款")
	}
	if e.Status != model.EpPaused {
		return 0, biz.NewBiz(biz.CodeBadState, "仅暂停观看期间的付费集可申请退款")
	}
	entID, ok, err := sv.activeEnt(u.ID, epID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, biz.NewBiz(biz.CodeBadState, "未购买该集或权益已退款，不能申请退款")
	}
	// 只退一次：存在 pending/approved 记录即拒绝
	var cnt int
	if err := sv.S.DB.QueryRow(`SELECT COUNT(*) FROM refunds WHERE user_id=? AND episode_id=? AND status IN (?,?)`,
		u.ID, epID, model.RefPending, model.RefApproved).Scan(&cnt); err != nil {
		return 0, err
	}
	if cnt > 0 {
		return 0, biz.NewBiz(biz.CodeDuplicate, "该集已存在待处理或已通过的退款申请，只能退一次")
	}
	r, err := sv.S.DB.Exec(`INSERT INTO refunds(user_id,episode_id,entitlement_id,amount,reason,status,created_at)
		VALUES(?,?,?,?,?,?,datetime('now','localtime'))`,
		u.ID, epID, entID, e.PubPrice, reason, model.RefPending)
	if err != nil {
		return 0, err
	}
	id, _ := r.LastInsertId()
	return id, nil
}

// PendingList 客服待处理退款列表（规则8：待处理退款可查）。
func (sv *Service) PendingList() ([]map[string]any, error) {
	rows, err := sv.S.DB.Query(`SELECT rf.id,rf.user_id,rf.episode_id,rf.amount,rf.reason,rf.created_at,e.pub_title,us.username
		FROM refunds rf JOIN episodes e ON e.id=rf.episode_id JOIN users us ON us.id=rf.user_id
		WHERE rf.status=? ORDER BY rf.id`, model.RefPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, uid, epID, amount int64
		var reason, at, title, uname string
		if err := rows.Scan(&id, &uid, &epID, &amount, &reason, &at, &title, &uname); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "user_id": uid, "username": uname, "episode_id": epID, "title": title,
			"amount": amount, "reason": reason, "created_at": at,
		})
	}
	return out, rows.Err()
}

// Decide 客服批准或驳回退款（规则6：确认后只退一次并撤销对应权益）。
// 批准动作在事务内完成：退币 + 权益置 refunded + 退款单置 approved，三方原子。
func (sv *Service) Decide(actor model.User, refundID int64, approve bool, rejectReason string) error {
	if err := biz.CheckRole(actor, model.RoleSupport); err != nil {
		return err
	}
	var uid, epID, entID, amount int64
	var status string
	err := sv.S.DB.QueryRow(`SELECT user_id,episode_id,entitlement_id,amount,status FROM refunds WHERE id=?`, refundID).
		Scan(&uid, &epID, &entID, &amount, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ErrNotFound
		}
		return err
	}
	if status != model.RefPending {
		return biz.NewBiz(biz.CodeBadState, "该退款申请已处理，不能重复处理")
	}
	if !approve {
		if err := biz.RequireNonEmpty(map[string]string{"驳回原因": rejectReason}); err != nil {
			return err
		}
		_, err := sv.S.DB.Exec(`UPDATE refunds SET status=?,reject_reason=?,processed_by=? WHERE id=? AND status=?`,
			model.RefRejected, rejectReason, actor.Username, refundID, model.RefPending)
		return err
	}
	tx, err := sv.S.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 只退一次：并发下仅一个事务能把 pending 置为 approved
	res, err := tx.Exec(`UPDATE refunds SET status=?,processed_by=? WHERE id=? AND status=?`,
		model.RefApproved, actor.Username, refundID, model.RefPending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return biz.NewBiz(biz.CodeDuplicate, "该退款申请已被处理，只退一次")
	}
	if _, err := tx.Exec(`UPDATE users SET balance=balance+? WHERE id=?`, amount, uid); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE entitlements SET status=?,refunded_at=? WHERE id=?`,
		model.EntRefunded, time.Now().Format("2006-01-02 15:04:05"), entID); err != nil {
		return err
	}
	return tx.Commit()
}

// Explain 客服核查：解释具体用户为何能看/不能看（规则8）。
type Explain struct {
	Username     string
	EpisodeID    int64
	Title        string
	EpStatus     string
	Balance      int64
	CanWatch     bool
	HasRight     bool
	Refunded     bool
	Reason       string
	Purchases    int64
	RefundStatus string // 无记录/待处理/已通过/已驳回
}

func (sv *Service) Explain(actor model.User, username string, epID int64) (*Explain, error) {
	if err := biz.CheckRole(actor, model.RoleSupport, model.RoleOps); err != nil {
		return nil, err
	}
	target, err := sv.B.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	e, err := sv.B.GetEpisode(epID)
	if err != nil {
		return nil, err
	}
	d, err := sv.B.Watch(target, epID)
	if err != nil {
		return nil, err
	}
	var purchases int
	_ = sv.S.DB.QueryRow(`SELECT COUNT(*) FROM purchases WHERE user_id=? AND episode_id=?`, target.ID, epID).Scan(&purchases)
	refStatus := "无记录"
	var rst string
	err = sv.S.DB.QueryRow(`SELECT status FROM refunds WHERE user_id=? AND episode_id=? ORDER BY id DESC LIMIT 1`,
		target.ID, epID).Scan(&rst)
	if err == nil {
		refStatus = rst
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &Explain{
		Username: username, EpisodeID: epID, Title: e.PubTitle, EpStatus: e.Status, Balance: target.Balance,
		CanWatch: d.Allowed, HasRight: d.HasRight, Refunded: d.Refunded, Reason: d.Reason,
		Purchases: int64(purchases), RefundStatus: refStatus,
	}, nil
}

func (sv *Service) activeEnt(userID, epID int64) (int64, bool, error) {
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
