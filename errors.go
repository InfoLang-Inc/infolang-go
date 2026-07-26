package infolang

import (
	"errors"
	"fmt"
)

// Sentinel errors let callers classify failures with errors.Is without matching
// on HTTP status codes. An *APIError reports these via its Is method.
var (
	// ErrAuthentication indicates a 401/403: the credential was missing,
	// invalid, or lacked permission (including lane_not_supported, which the
	// gateway reports as 403).
	ErrAuthentication = errors.New("infolang: authentication failed")
	// ErrNotFound indicates a 404: the workspace, namespace, or memory id
	// does not exist.
	ErrNotFound = errors.New("infolang: not found")
	// ErrValidation indicates a 400/422: the request payload was rejected.
	ErrValidation = errors.New("infolang: validation error")
	// ErrRateLimit indicates a 429: quota exceeded.
	ErrRateLimit = errors.New("infolang: rate limited")
	// ErrServer indicates a 5xx: the gateway failed to process the request.
	ErrServer = errors.New("infolang: server error")
)

// ConfigError is returned for client misconfiguration such as missing
// credentials, a bad base URL, or an unresolvable workspace.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string { return "infolang: " + e.Message }

// ConnectionError is returned when the gateway could not be reached or the
// request timed out. The underlying transport error is wrapped.
type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("infolang: connection error: %v", e.Err)
}

func (e *ConnectionError) Unwrap() error { return e.Err }

// APIError is returned when the gateway responds with a non-2xx status. The
// raw StatusCode, decoded Body, RequestID, and machine-readable Code are
// always available. Use errors.Is with the sentinel errors above to branch on
// well-known statuses.
type APIError struct {
	StatusCode int
	Message    string
	// Code is the gateway's machine-readable error code from the /v2
	// {"error":{code,message}} envelope, or "" when absent.
	Code      string
	Body      any
	RequestID string
	// RetryAfter is the server-advised delay in seconds for a 429, or 0.
	RetryAfter float64
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("infolang: %s (status=%d request_id=%s)", e.Message, e.StatusCode, e.RequestID)
	}
	return fmt.Sprintf("infolang: %s (status=%d)", e.Message, e.StatusCode)
}

// Is maps the HTTP status code onto the exported sentinel errors so callers can
// write errors.Is(err, infolang.ErrNotFound).
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrAuthentication:
		return e.StatusCode == 401 || e.StatusCode == 403
	case ErrNotFound:
		return e.StatusCode == 404
	case ErrValidation:
		return e.StatusCode == 400 || e.StatusCode == 422
	case ErrRateLimit:
		return e.StatusCode == 429
	case ErrServer:
		return e.StatusCode >= 500
	}
	return false
}

// errorFromResponse builds the most specific *APIError for a status code.
func errorFromResponse(status int, body any, requestID string, retryAfter float64) *APIError {
	msg := messageFromBody(body)
	if msg == "" {
		msg = fmt.Sprintf("request failed with status %d", status)
	}
	return &APIError{
		StatusCode: status,
		Message:    msg,
		Code:       errorCode(body),
		Body:       body,
		RequestID:  requestID,
		RetryAfter: retryAfter,
	}
}

// messageFromBody pulls a human message out of a decoded error body. The
// gateway /v2 envelope {"error":{code,message}} renders as "code: message";
// flat error/message/detail keys are the fallback.
func messageFromBody(body any) string {
	if m, ok := body.(map[string]any); ok {
		if nested, ok := m["error"].(map[string]any); ok {
			message, _ := nested["message"].(string)
			code, _ := nested["code"].(string)
			if message != "" {
				if code != "" {
					return code + ": " + message
				}
				return message
			}
			if code != "" {
				return code
			}
		}
		for _, key := range []string{"error", "message", "detail"} {
			if v, ok := m[key].(string); ok && v != "" {
				return v
			}
		}
	}
	if s, ok := body.(string); ok {
		return s
	}
	return ""
}

// errorCode pulls the gateway's machine-readable error code
// ({"error":{code}}) out of a decoded error body, or "".
func errorCode(body any) string {
	if m, ok := body.(map[string]any); ok {
		if nested, ok := m["error"].(map[string]any); ok {
			if code, ok := nested["code"].(string); ok {
				return code
			}
		}
	}
	return ""
}
