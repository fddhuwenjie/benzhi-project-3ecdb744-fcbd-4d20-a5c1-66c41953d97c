package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"shelter-drill-gate/internal/domain"
)

const maxRequestBody = 1 << 20

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code            string              `json:"code"`
	Message         string              `json:"message"`
	Fields          []domain.FieldError `json:"fields,omitempty"`
	CurrentRevision int64               `json:"current_revision,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.Invalid("Content-Type", "必须是 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Invalid("body", "JSON 请求体无效："+err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Invalid("body", "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	body := errorBody{Code: "internal_error", Message: "服务处理请求时发生错误"}
	var validation *domain.ValidationError
	var conflict *domain.RevisionConflict
	switch {
	case errors.As(err, &validation):
		status, body.Code, body.Message, body.Fields = http.StatusUnprocessableEntity, "validation_error", validation.Error(), validation.Fields
	case errors.As(err, &conflict):
		status, body.Code, body.Message, body.CurrentRevision = http.StatusConflict, "revision_conflict", conflict.Error(), conflict.Current
	case errors.Is(err, domain.ErrNotFound):
		status, body.Code, body.Message = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, domain.ErrForbidden):
		status, body.Code, body.Message = http.StatusForbidden, "forbidden", "协调员与独立复核员必须职责分离，或当前角色无权执行该操作"
	}
	writeJSON(w, status, errorEnvelope{Error: body})
}
