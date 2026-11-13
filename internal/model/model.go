// Package model 定义题目04短剧业务的数据模型。
package model

// 角色常量
const (
	RoleEditor   = "editor"   // 内容编辑
	RoleReviewer = "reviewer" // 审核人员
	RoleOps      = "ops"      // 运营
	RoleUser     = "user"     // 用户
	RoleSupport  = "support"  // 客服
)

// 剧集状态
const (
	EpDraft         = "draft"          // 草稿
	EpPendingReview = "pending_review" // 待审核
	EpRejected      = "rejected"       // 审核退回
	EpApproved      = "approved"       // 审核通过（未上架）
	EpPublished     = "published"      // 已上架
	EpSalesStopped  = "sales_stopped"  // 停止销售（已购可看）
	EpPaused        = "paused"         // 暂停观看（禁止购买与播放）
)

// 修订状态
const (
	RevPending  = "pending"
	RevApproved = "approved"
	RevRejected = "rejected"
)

// 权益状态
const (
	EntActive   = "active"
	EntRefunded = "refunded"
)

// 退款状态
const (
	RefPending  = "pending"
	RefApproved = "approved"
	RefRejected = "rejected"
)

// User 预置账号。
type User struct {
	ID       int64
	Username string
	Role     string
	Balance  int64 // 平台币
}

// Episode 剧集：cur_* 为编辑当前工作稿，pub_* 为线上已发布版本。
type Episode struct {
	ID           int64
	Seq          int
	Status       string
	CurTitle     string
	CurIsFree    bool
	CurPrice     int64
	CurPlayURL   string
	PubTitle     string
	PubIsFree    bool
	PubPrice     int64
	PubPlayURL   string
	SubmittedBy  string
	RejectReason string
}

// Revision 已上架内容的待审修订（每集最多一份待审）。
type Revision struct {
	ID           int64
	EpisodeID    int64
	Title        string
	IsFree       bool
	PlayURL      string
	Status       string
	RejectReason string
	SubmittedBy  string
	ReviewedBy   string
}

// Purchase 购买记录（历史只增不改）。
type Purchase struct {
	ID        int64
	RequestID string
	UserID    int64
	EpisodeID int64
	Price     int64
	CreatedAt string
}

// Entitlement 单集观看权益。
type Entitlement struct {
	ID         int64
	UserID     int64
	EpisodeID  int64
	PurchaseID int64
	Status     string
	CreatedAt  string
}

// RefundRequest 暂停观看期间的退款申请。
type RefundRequest struct {
	ID            int64
	RequestID     string
	UserID        int64
	EpisodeID     int64
	EntitlementID int64
	Amount        int64
	Reason        string
	Status        string
	RejectReason  string
	ProcessedBy   string
	CreatedAt     string
}

// OpLog 运营/审核操作留痕。
type OpLog struct {
	ID        int64
	EpisodeID int64
	Action    string
	Reason    string
	Actor     string
	CreatedAt string
}
