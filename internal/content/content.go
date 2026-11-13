// Package content 实现剧集内容资料维护、提交审核与审核工作流（题面规则1/2）。
package content

import (
	"database/sql"
	"errors"

	"aitv/internal/biz"
	"aitv/internal/model"
	"aitv/internal/store"
)

// Service 内容工作流服务。
type Service struct {
	S *store.Store
}

func New(s *store.Store) *Service { return &Service{S: s} }

const nowFunc = "datetime('now','localtime')"

// GetEpisode 按编号取剧集。
func (sv *Service) GetEpisode(id int64) (model.Episode, error) {
	row := sv.S.DB.QueryRow(`SELECT id,seq,status,cur_title,cur_is_free,cur_price,cur_play_url,
		pub_title,pub_is_free,pub_price,pub_play_url,submitted_by,reject_reason
		FROM episodes WHERE id=?`, id)
	return scanEpisode(row)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanEpisode(r rowScanner) (model.Episode, error) {
	var e model.Episode
	var cf, pf int
	err := r.Scan(&e.ID, &e.Seq, &e.Status, &e.CurTitle, &cf, &e.CurPrice, &e.CurPlayURL,
		&e.PubTitle, &pf, &e.PubPrice, &e.PubPlayURL, &e.SubmittedBy, &e.RejectReason)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, biz.ErrNotFound
		}
		return e, err
	}
	e.CurIsFree = cf == 1
	e.PubIsFree = pf == 1
	return e, nil
}

// ListEpisodes 全量剧集（按集序）。
func (sv *Service) ListEpisodes() ([]model.Episode, error) {
	rows, err := sv.S.DB.Query(`SELECT id,seq,status,cur_title,cur_is_free,cur_price,cur_play_url,
		pub_title,pub_is_free,pub_price,pub_play_url,submitted_by,reject_reason FROM episodes ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Episode
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SubmitForReview 编辑提交审核（规则1：维护资料并提交；已上架内容走修订，规则2）。
// 返回 (是否为修订提交, 错误)。
func (sv *Service) SubmitForReview(actor model.User, epID int64, title string, isFree bool, playURL string) (bool, error) {
	if err := biz.CheckRole(actor, model.RoleEditor); err != nil {
		return false, err
	}
	if err := biz.RequireNonEmpty(map[string]string{"标题": title, "播放地址": playURL}); err != nil {
		return false, err
	}
	e, err := sv.GetEpisode(epID)
	if err != nil {
		return false, err
	}
	// 价格在首次上架后固定：已发布过的内容修订不可改免费/付费属性与价格
	isRevision := e.Status == model.EpPublished || e.Status == model.EpSalesStopped || e.Status == model.EpPaused
	if isRevision && isFree != e.PubIsFree {
		return false, biz.NewBiz(biz.CodeBadRequest, "已上架内容不可修改免费/付费属性")
	}

	if isRevision {
		// 每集最多一份待审修订
		var cnt int
		if err := sv.S.DB.QueryRow(`SELECT COUNT(*) FROM revisions WHERE episode_id=? AND status=?`,
			epID, model.RevPending).Scan(&cnt); err != nil {
			return false, err
		}
		if cnt > 0 {
			return false, biz.NewBiz(biz.CodeDuplicate, "该集已存在待审修订，须待其处理后再提交")
		}
		if _, err := sv.S.DB.Exec(`INSERT INTO revisions(episode_id,title,is_free,play_url,status,submitted_by)
			VALUES(?,?,?,?,?,?)`, epID, title, boolInt(isFree), playURL, model.RevPending, actor.Username); err != nil {
			return false, err
		}
		if err := sv.log(epID, "提交修订", actor.Username, ""); err != nil {
			return false, err
		}
		return true, nil
	}

	// 未上架内容：draft/rejected/approved 均可修改并提交审核
	switch e.Status {
	case model.EpDraft, model.EpRejected, model.EpApproved, model.EpPendingReview:
	default:
		return false, biz.NewBiz(biz.CodeBadState, "当前状态不可提交审核")
	}
	price := int64(0)
	if !isFree {
		price = e.CurPrice
		if price <= 0 {
			return false, biz.NewBiz(biz.CodeBadRequest, "付费集价格必须大于0")
		}
	}
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET cur_title=?,cur_is_free=?,cur_price=?,cur_play_url=?,
		status=?,submitted_by=?,reject_reason='' WHERE id=?`,
		title, boolInt(isFree), price, playURL, model.EpPendingReview, actor.Username, epID); err != nil {
		return false, err
	}
	if err := sv.log(epID, "提交审核", actor.Username, ""); err != nil {
		return false, err
	}
	return false, nil
}

// Review 审核人员通过或退回（规则1）；编辑不可自审；同时处理剧集与待审修订两类对象。
func (sv *Service) Review(actor model.User, target string, targetID int64, approve bool, reason string) error {
	if err := biz.CheckRole(actor, model.RoleReviewer); err != nil {
		return err
	}
	switch target {
	case "episode":
		return sv.reviewEpisode(actor, targetID, approve, reason)
	case "revision":
		return sv.reviewRevision(actor, targetID, approve, reason)
	default:
		return biz.ErrBadRequest
	}
}

func (sv *Service) reviewEpisode(actor model.User, epID int64, approve bool, reason string) error {
	e, err := sv.GetEpisode(epID)
	if err != nil {
		return err
	}
	if e.Status != model.EpPendingReview {
		return biz.NewBiz(biz.CodeBadState, "该集不在待审核状态")
	}
	if e.SubmittedBy == actor.Username {
		return biz.ErrSelfReview
	}
	newStatus := model.EpApproved
	if !approve {
		if err := biz.RequireNonEmpty(map[string]string{"退回原因": reason}); err != nil {
			return err
		}
		newStatus = model.EpRejected
	}
	if _, err := sv.S.DB.Exec(`UPDATE episodes SET status=?,reject_reason=? WHERE id=?`,
		newStatus, reason, epID); err != nil {
		return err
	}
	action := "审核通过"
	if !approve {
		action = "审核退回"
	}
	return sv.log(epID, action, actor.Username, reason)
}

func (sv *Service) reviewRevision(actor model.User, revID int64, approve bool, reason string) error {
	row := sv.S.DB.QueryRow(`SELECT id,episode_id,title,is_free,play_url,status,reject_reason,submitted_by,reviewed_by
		FROM revisions WHERE id=?`, revID)
	var r model.Revision
	var isFree int
	err := row.Scan(&r.ID, &r.EpisodeID, &r.Title, &isFree, &r.PlayURL, &r.Status, &r.RejectReason, &r.SubmittedBy, &r.ReviewedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ErrNotFound
		}
		return err
	}
	r.IsFree = isFree == 1
	if r.Status != model.RevPending {
		return biz.NewBiz(biz.CodeBadState, "该修订不在待审核状态")
	}
	if r.SubmittedBy == actor.Username {
		return biz.ErrSelfReview
	}
	newStatus := model.RevApproved
	if !approve {
		if err := biz.RequireNonEmpty(map[string]string{"退回原因": reason}); err != nil {
			return err
		}
		newStatus = model.RevRejected
	}
	if _, err := sv.S.DB.Exec(`UPDATE revisions SET status=?,reject_reason=?,reviewed_by=? WHERE id=?`,
		newStatus, reason, actor.Username, revID); err != nil {
		return err
	}
	action := "修订审核通过"
	if !approve {
		action = "修订审核退回"
	}
	return sv.log(r.EpisodeID, action, actor.Username, reason)
}

// ListPendingReviews 待审核剧集（不含提交人，防自审）。
func (sv *Service) ListPendingReviews() ([]model.Episode, error) {
	rows, err := sv.S.DB.Query(`SELECT id,seq,status,cur_title,cur_is_free,cur_price,cur_play_url,
		pub_title,pub_is_free,pub_price,pub_play_url,submitted_by,reject_reason
		FROM episodes WHERE status=? ORDER BY seq`, model.EpPendingReview)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Episode
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListPendingRevisions 待审修订（附剧集标题）。
func (sv *Service) ListPendingRevisions() ([]map[string]any, error) {
	rows, err := sv.S.DB.Query(`SELECT r.id,r.episode_id,r.title,r.is_free,r.play_url,r.submitted_by,e.pub_title
		FROM revisions r JOIN episodes e ON e.id=r.episode_id WHERE r.status=? ORDER BY r.id`, model.RevPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, epID, isFree int64
		var title, playURL, by, epTitle string
		if err := rows.Scan(&id, &epID, &title, &isFree, &playURL, &by, &epTitle); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "episode_id": epID, "title": title, "is_free": isFree == 1,
			"play_url": playURL, "submitted_by": by, "episode_title": epTitle,
		})
	}
	return out, rows.Err()
}

// ListApprovedRevisions 已通过待运营发布的修订。
func (sv *Service) ListApprovedRevisions() ([]map[string]any, error) {
	rows, err := sv.S.DB.Query(`SELECT r.id,r.episode_id,r.title,r.is_free,r.play_url
		FROM revisions r JOIN episodes e ON e.id=r.episode_id
		WHERE r.status=? AND e.status IN (?,?,?) ORDER BY r.id`,
		model.RevApproved, model.EpPublished, model.EpSalesStopped, model.EpPaused)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, epID, isFree int64
		var title, playURL string
		if err := rows.Scan(&id, &epID, &title, &isFree, &playURL); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "episode_id": epID, "title": title, "is_free": isFree == 1, "play_url": playURL,
		})
	}
	return out, rows.Err()
}

func (sv *Service) log(epID int64, action, actor, reason string) error {
	_, err := sv.S.DB.Exec(`INSERT INTO op_logs(episode_id,action,reason,actor,created_at)
		VALUES(?,?,?,?,`+nowFunc+`)`, epID, action, reason, actor)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
