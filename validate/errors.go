package validate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ─── Error types ─────────────────────────────────────────────────────────────

// FieldError is a single field validation failure.
type FieldError struct {
	Field   string // display name from json/form/uri/query tag, or struct field name
	Tag     string // validation rule that failed (e.g. "required", "email", "min")
	Param   string // parameter for the rule (e.g. "8" for min=8)
	Value   any    // the value that failed
	Message string // human-readable description
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Errors is a collection of field-level validation failures.
// It implements the error interface and can be converted to a JSON-friendly map.
type Errors []*FieldError

// Error returns a semicolon-separated summary of all failures.
func (e Errors) Error() string {
	msgs := make([]string, len(e))
	for i, fe := range e {
		msgs[i] = fe.Error()
	}
	return strings.Join(msgs, "; ")
}

// Map returns a field→message map, suitable for JSON API error responses.
//
//	errs.Map() // map["email":"请输入有效的电子邮箱地址" "name":"此字段为必填项"]
func (e Errors) Map() map[string]string {
	m := make(map[string]string, len(e))
	for _, fe := range e {
		m[fe.Field] = fe.Message
	}
	return m
}

// First returns the first FieldError, or nil if e is empty.
func (e Errors) First() *FieldError {
	if len(e) == 0 {
		return nil
	}
	return e[0]
}

// ─── Internal translation ─────────────────────────────────────────────────────

// toErrors converts a validator error into our Errors type.
func toErrors(err error) error {
	var verr validator.ValidationErrors
	if !errors.As(err, &verr) {
		return Errors{{Field: "_", Tag: "internal", Message: err.Error()}}
	}
	out := make(Errors, len(verr))
	for i, fe := range verr {
		out[i] = &FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Param:   fe.Param(),
			Value:   fe.Value(),
			Message: buildMessage(fe),
		}
	}
	return out
}

// buildMessage returns a human-readable (Chinese) validation message.
func buildMessage(fe validator.FieldError) string {
	p := fe.Param()
	kind := fe.Kind()
	isCollection := kind == reflect.String || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Array

	switch fe.Tag() {
	// ── Required ──
	case "required", "required_if", "required_with", "required_without",
		"required_unless", "required_with_all", "required_without_all":
		return "此字段为必填项"

	// ── Format ──
	case "email":
		return "请输入有效的电子邮箱地址"
	case "url", "uri", "http_url":
		return "请输入有效的 URL"
	case "uuid", "uuid3", "uuid4", "uuid5":
		return "请输入有效的 UUID"
	case "ip":
		return "请输入有效的 IP 地址"
	case "ipv4", "ip4_addr":
		return "请输入有效的 IPv4 地址"
	case "ipv6", "ip6_addr":
		return "请输入有效的 IPv6 地址"
	case "e164":
		return "请输入有效的 E.164 格式电话号码"
	case "json":
		return "必须是有效的 JSON 字符串"
	case "base64":
		return "必须是有效的 Base64 编码"
	case "datetime":
		return fmt.Sprintf("请按 %s 格式填写日期时间", p)

	// ── Length & range ──
	case "min":
		if isCollection {
			return fmt.Sprintf("长度不能少于 %s 个字符", p)
		}
		return fmt.Sprintf("数值不能小于 %s", p)
	case "max":
		if isCollection {
			return fmt.Sprintf("长度不能超过 %s 个字符", p)
		}
		return fmt.Sprintf("数值不能大于 %s", p)
	case "len":
		if isCollection {
			return fmt.Sprintf("长度必须为 %s", p)
		}
		return fmt.Sprintf("必须等于 %s", p)
	case "gt":
		return fmt.Sprintf("必须大于 %s", p)
	case "gte":
		return fmt.Sprintf("不能小于 %s", p)
	case "lt":
		return fmt.Sprintf("必须小于 %s", p)
	case "lte":
		return fmt.Sprintf("不能大于 %s", p)
	case "eq":
		return fmt.Sprintf("必须等于 %s", p)
	case "ne":
		return fmt.Sprintf("不能等于 %s", p)

	// ── Enum & content ──
	case "oneof":
		return fmt.Sprintf("必须是以下值之一：%s", strings.ReplaceAll(p, " ", "、"))
	case "contains":
		return fmt.Sprintf("必须包含 %q", p)
	case "excludes":
		return fmt.Sprintf("不能包含 %q", p)
	case "startswith":
		return fmt.Sprintf("必须以 %q 开头", p)
	case "endswith":
		return fmt.Sprintf("必须以 %q 结尾", p)
	case "alpha":
		return "只能包含英文字母"
	case "alphanum":
		return "只能包含英文字母和数字"
	case "numeric":
		return "必须是数字字符串"
	case "number":
		return "必须是有效数字"

	// ── Built-in custom validators ──
	case "mobile":
		return "请输入有效的手机号码"
	case "password":
		return "密码强度不足：至少 8 位，须包含大写字母、小写字母、数字及特殊字符"
	case "username":
		return "用户名只能包含字母、数字和下划线，长度 3–32 位"
	case "no_html":
		return "不能包含 HTML 标签"
	case "not_blank":
		return "不能为纯空白字符"

	default:
		return fmt.Sprintf("字段验证失败（规则：%s）", fe.Tag())
	}
}
