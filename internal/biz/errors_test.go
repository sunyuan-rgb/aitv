package biz

import (
	"errors"
	"testing"

	"aitv/internal/model"
)

func TestBizError(t *testing.T) {
	e := NewBiz("x", "消息")
	if e.Error() != "消息" {
		t.Fatal("Error() 不符")
	}
	var target error = e
	b, ok := AsBiz(target)
	if !ok || b.Code != "x" {
		t.Fatalf("AsBiz: %v %v", b, ok)
	}
	// 包装后仍可识别
	wrapped := errors.New("外层: " + target.Error())
	if _, ok := AsBiz(wrapped); ok {
		t.Fatal("非业务错误不应识别")
	}
	if _, ok := AsBiz(errors.Join(errors.New("a"), target)); !ok {
		t.Fatal("errors.Join 后应可识别")
	}
}

func TestCheckRole(t *testing.T) {
	u := model.User{Role: model.RoleOps}
	if err := CheckRole(u, model.RoleOps, model.RoleSupport); err != nil {
		t.Fatal("匹配角色应通过")
	}
	if err := CheckRole(u, model.RoleUser); err != ErrForbidden {
		t.Fatalf("不匹配应返回 ErrForbidden: %v", err)
	}
}

func TestRequireNonEmpty(t *testing.T) {
	if err := RequireNonEmpty(map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatal("全非空应通过")
	}
	err := RequireNonEmpty(map[string]string{"a": "", "b": "2"})
	if err == nil || err.Error() != "a不能为空" {
		t.Fatalf("空字段报错: %v", err)
	}
}

func TestFormatErr(t *testing.T) {
	e := FormatErr("code1", "状态为%s", "暂停")
	if e.Code != "code1" || e.Msg != "状态为暂停" {
		t.Fatalf("FormatErr: %+v", e)
	}
}
