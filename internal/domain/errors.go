package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound  = errors.New("资源不存在")
	ErrConflict  = errors.New("数据版本冲突")
	ErrForbidden = errors.New("无权执行该操作")
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "输入校验失败"
	}
	return fmt.Sprintf("%s: %s", e.Fields[0].Field, e.Fields[0].Message)
}

func Invalid(field, message string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}

func IsValidation(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}

type RevisionConflict struct {
	Expected int64 `json:"expected_revision"`
	Current  int64 `json:"current_revision"`
}

func (e *RevisionConflict) Error() string {
	return fmt.Sprintf("%v：期望 %d，当前 %d", ErrConflict, e.Expected, e.Current)
}
