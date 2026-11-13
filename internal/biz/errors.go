// Package biz 定义跨包共享的业务错误与鉴权助手。
package biz

import (
	"errors"
	"fmt"

	"aitv/internal/model"
)

// Biz 业务错误：Code 用于接口层映射，Msg 为用户可读原因。
type Biz struct {
	Code string
	Msg  string
}

func (e *Biz) Error() string { return e.Msg }

// 常用错误码
const (
	CodeForbidden  = "forbidden"
	CodeNotFound   = "not_found"
	CodeBadState   = "bad_state"
	CodeBadRequest = "bad_request"
	CodeNoBalance  = "insufficient_balance"
	CodeDuplicate  = "duplicate"
	CodeConflict   = "conflict"
)

func NewBiz(code, msg string) *Biz { return &Biz{Code: code, Msg: msg} }

// IsBiz 判断错误是否为业务错误并取出。
func AsBiz(err error) (*Biz, bool) {
	var b *Biz
	if errors.As(err, &b) {
		return b, true
	}
	return nil, false
}

// 常见预置错误
var (
	ErrForbidden   = NewBiz(CodeForbidden, "无权限执行该操作")
	ErrNotLoggedIn = NewBiz(CodeForbidden, "未登录")
	ErrNotFound    = NewBiz(CodeNotFound, "记录不存在")
	ErrSelfReview  = NewBiz(CodeForbidden, "不能审核自己提交的内容")
	ErrBadRequest  = NewBiz(CodeBadRequest, "请求参数不合法")
)

// CheckRole 校验操作者角色，不符返回 ErrForbidden。
func CheckRole(u model.User, roles ...string) error {
	for _, r := range roles {
		if u.Role == r {
			return nil
		}
	}
	return ErrForbidden
}

// RequireNonEmpty 校验必填字段。
func RequireNonEmpty(fields map[string]string) error {
	for name, v := range fields {
		if v == "" {
			return NewBiz(CodeBadRequest, name+"不能为空")
		}
	}
	return nil
}

// FormatErr 包装带上下文的业务错误。
func FormatErr(code string, format string, args ...any) *Biz {
	return NewBiz(code, fmt.Sprintf(format, args...))
}
