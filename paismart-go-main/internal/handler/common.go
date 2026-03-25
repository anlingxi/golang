package handler

import "pai-smart-go/pkg/code"

// 统一响应结构体
type Response struct {
	StatusCode code.Code `json:"status_code"`
	StatusMsg  string    `json:"status_msg,omitempty"`
}

// SetCode 设置响应的状态码和消息。
func (r *Response) SetCode(c code.Code) Response {
	if nil == r {
		r = new(Response)
	}
	r.StatusCode = c
	r.StatusMsg = c.Msg()
	return *r
}

func (r *Response) Success() {
	r.SetCode(code.CodeSuccess)
}
