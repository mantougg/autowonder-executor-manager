// errors.go：REST API 统一错误响应（T06-F3）。
// 契约（记入 T06 变更记录，T07 CLI / T09 Web 依赖）：
//
//	{"error":{"code":"<机器码>","message":"<中文>","field":"<可选，逗号分隔>"}}
//
// code 与 HTTP 状态的映射固定：bad_request/invalid_name/invalid_command/
// invalid_config → 400；unauthorized → 401；forbidden → 403；not_found → 404；
// already_exists/running → 409；internal → 500。
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"awem/internal/launchcmd"
)

// apiError 是携带 HTTP 状态与机器码的 API 错误。
type apiError struct {
	Status  int
	Code    string
	Message string
	Field   string // 可选：逐字段定位（逗号分隔的 flag/字段名）
}

// Error 实现 error。
func (e *apiError) Error() string { return e.Message }

// 错误构造器。
func badRequest(msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}
func invalidName(msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "invalid_name", Message: msg, Field: "name"}
}
func notFound(msg string) *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}
func alreadyExists(name string) *apiError {
	return &apiError{Status: http.StatusConflict, Code: "already_exists",
		Message: "执行器 " + name + " 已存在（重复 add 不覆盖）", Field: "name"}
}
func internalErr(err error) *apiError {
	return &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
}

// commandError 把命令解析/校验错误转为逐字段 API 错误（FieldErrors 聚合）。
func commandError(err error) *apiError {
	var fe launchcmd.FieldErrors
	if errors.As(err, &fe) {
		flags := make([]string, 0, len(fe))
		msgs := make([]string, 0, len(fe))
		for _, issue := range fe {
			flags = append(flags, issue.Flag)
			msgs = append(msgs, issue.Error())
		}
		return &apiError{
			Status: http.StatusBadRequest, Code: "invalid_command",
			Message: "命令字段校验失败：" + joinCN(msgs), Field: joinComma(flags),
		}
	}
	return &apiError{Status: http.StatusBadRequest, Code: "invalid_command", Message: err.Error()}
}

// writeJSON 输出 JSON（Content-Type 固定；nil data 不输出 body）。
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
}

// writeAPIError 按统一 schema 输出错误。
func writeAPIError(w http.ResponseWriter, e *apiError) {
	field := e.Field
	if field == "" {
		// 稳定 schema：field 缺省时输出空串（字段存在，便于消费方统一读取）
		writeJSON(w, e.Status, map[string]any{"error": map[string]any{
			"code": e.Code, "message": e.Message, "field": ""}})
		return
	}
	writeJSON(w, e.Status, map[string]any{"error": map[string]any{
		"code": e.Code, "message": e.Message, "field": field}})
}

// joinCN 用中文分号连接。
func joinCN(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += "；"
		}
		out += s
	}
	return out
}

// joinComma 用逗号连接（field 字段的多值形态）。
func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
