// Package sales 实现运营上架、停止销售、暂停观看、恢复及影响范围查询（题面规则5/8）。
package sales

import (
	"aitv/internal/biz"
	"aitv/internal/content"
	"aitv/internal/model"
	"aitv/internal/store"
)

// Service 运营内容管理服务。
type Service struct {
	S *store.Store
	C *content.Service
}

func New(s *store.Store, c *content.Service) *Service { return &Service{S: s, C: c} }

// Publish 首次上架：仅审核通过内容可上架；上架后价格固定（规则4）。
func (sv *Service) Publish(actor model.User, epID int64) error {
	if err := biz.CheckRole(actor, model.RoleOps); err != nil {
		return err
	}
	e, err := sv.C.GetEpisode(epID)
	if err != nil {
		return err
	}
	if e.Status != model.EpApproved {
		return biz.NewBiz(biz.CodeBadState, "仅审核通过的内容可上架")
	}
	_, err = sv.S.DB.Exec(`UPDATE episodes SET status=?,pub_title=?,pub_is_free=?,pub_price=?,pub_play_url=?
		WHERE id=?`, model.EpPublished, e.CurTitle, boolInt(e.CurIsFree), e.CurPrice, e.CurPlayURL, epID)
	if err != nil {
		return err
	}
	return sv.log(epID, "上架", actor.Username, "")
}

// PublishRevision 发布已审核通过的修订，替换线上版本（未公开修订在此刻才对外生效）。
func (sv *Service) PublishRevision(actor model.User, revID int64) error {
	if err := biz.CheckRole(actor, model.RoleOps); err != nil {
		return err
	}
	row := sv.S.DB.QueryRow(`SELECT episode_id,title,is_free,play_url,status FROM revisions WHERE id=?`, revID)
	var epID int64
	var title, playURL, status string
	var isFree int
	if err := row.Scan(&epID, &title, &isFree, &playURL, &status); err != nil {
		return biz.ErrNotFound
	}
	if status != model.RevApproved {
		return biz.NewBiz(biz.CodeBadState, "修订未审核通过，不能发布")
	}
	var epStatus string
	if err := sv.S.DB.QueryRow(`SELECT status FROM episodes WHERE id=?`, epID).Scan(&epStatus); err != nil {
		return biz.ErrNotFound
	}
	if epStatus != model.EpPublished && epStatus != model.EpSalesStopped && epStatus != model.EpPaused {
		return biz.NewBiz(biz.CodeBadState, "剧集未处于已上架状态，不能发布修订")
	}
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET pub_title=?,pub_play_url=? WHERE id=?`,
		title, playURL, epID); err != nil {
		return err
	}
	return sv.log(epID, "发布修订", actor.Username, title)
}

// StopSales 停止销售：仅禁止新购买，已购用户仍可观看；须填写原因（规则5）。
func (sv *Service) StopSales(actor model.User, epID int64, reason string) error {
	return sv.transition(actor, epID, model.EpPublished, model.EpSalesStopped, "停止销售", reason)
}

// Pause 暂停观看：同时禁止购买与播放，含已购用户；须填写原因（规则5）。
func (sv *Service) Pause(actor model.User, epID int64, reason string) error {
	return sv.transition(actor, epID, model.EpPublished, model.EpPaused, "暂停观看", reason)
}

// Resume 恢复：停止销售或暂停观看回到已上架（规则7：恢复可用后未退款用户恢复观看）。
func (sv *Service) Resume(actor model.User, epID int64, reason string) error {
	if err := biz.CheckRole(actor, model.RoleOps); err != nil {
		return err
	}
	if err := biz.RequireNonEmpty(map[string]string{"恢复原因": reason}); err != nil {
		return err
	}
	e, err := sv.C.GetEpisode(epID)
	if err != nil {
		return err
	}
	if e.Status != model.EpSalesStopped && e.Status != model.EpPaused {
		return biz.NewBiz(biz.CodeBadState, "仅停止销售或暂停观看的内容可恢复")
	}
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET status=? WHERE id=?`, model.EpPublished, epID); err != nil {
		return err
	}
	return sv.log(epID, "恢复", actor.Username, reason)
}

func (sv *Service) transition(actor model.User, epID int64, from, to, action, reason string) error {
	if err := biz.CheckRole(actor, model.RoleOps); err != nil {
		return err
	}
	if err := biz.RequireNonEmpty(map[string]string{"操作原因": reason}); err != nil {
		return err
	}
	res, err := sv.S.DB.Exec(`UPDATE episodes SET status=? WHERE id=? AND status=?`, to, epID, from)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		e, err := sv.C.GetEpisode(epID)
		if err != nil {
			return err
		}
		return biz.FormatErr(biz.CodeBadState, "当前状态为%s，不允许该操作", statusText(e.Status))
	}
	return sv.log(epID, action, actor.Username, reason)
}

// EpisodeSummary 运营视角单集影响范围：状态、购买人数、待处理退款数。
type EpisodeSummary struct {
	Episode       model.Episode
	BuyerCount    int64 // 累计购买人数（含已退款，历史保留）
	ActiveHolders int64 // 当前有效权益人数
	PendingRefs   int64 // 待处理退款数
	LastAction    string
	LastReason    string
}

// OpsSummary 全部剧集的运营汇总（规则8）。
func (sv *Service) OpsSummary() ([]EpisodeSummary, error) {
	eps, err := sv.C.ListEpisodes()
	if err != nil {
		return nil, err
	}
	out := make([]EpisodeSummary, 0, len(eps))
	for _, e := range eps {
		var buyers, holders, pend int64
		_ = sv.S.DB.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM purchases WHERE episode_id=?`, e.ID).Scan(&buyers)
		_ = sv.S.DB.QueryRow(`SELECT COUNT(*) FROM entitlements WHERE episode_id=? AND status=?`, e.ID, model.EntActive).Scan(&holders)
		_ = sv.S.DB.QueryRow(`SELECT COUNT(*) FROM refunds WHERE episode_id=? AND status=?`, e.ID, model.RefPending).Scan(&pend)
		var act, rs string
		_ = sv.S.DB.QueryRow(`SELECT action,reason FROM op_logs WHERE episode_id=? ORDER BY id DESC LIMIT 1`, e.ID).Scan(&act, &rs)
		out = append(out, EpisodeSummary{Episode: e, BuyerCount: buyers, ActiveHolders: holders, PendingRefs: pend, LastAction: act, LastReason: rs})
	}
	return out, nil
}

// RecentLogs 运营操作留痕（最近 N 条）。
func (sv *Service) RecentLogs(limit int) ([]model.OpLog, error) {
	rows, err := sv.S.DB.Query(`SELECT id,episode_id,action,reason,actor,created_at FROM op_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OpLog
	for rows.Next() {
		var l model.OpLog
		if err := rows.Scan(&l.ID, &l.EpisodeID, &l.Action, &l.Reason, &l.Actor, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (sv *Service) log(epID int64, action, actor, reason string) error {
	_, err := sv.S.DB.Exec(`INSERT INTO op_logs(episode_id,action,reason,actor,created_at)
		VALUES(?,?,?,?,datetime('now','localtime'))`, epID, action, reason, actor)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func statusText(s string) string {
	switch s {
	case model.EpPublished:
		return "已上架"
	case model.EpSalesStopped:
		return "停止销售"
	case model.EpPaused:
		return "暂停观看"
	case model.EpApproved:
		return "审核通过待上架"
	case model.EpPendingReview:
		return "待审核"
	case model.EpRejected:
		return "审核退回"
	default:
		return "草稿"
	}
}
