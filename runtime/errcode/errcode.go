package errcode

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/samber/oops"
)

// Problem is an RFC 9457 (Problem Details for HTTP APIs) document: the error
// body every service answers with, served as application/problem+json.
//
// `code` is a registered extension carrying the machine-readable error code;
// a client that only knows the standard ignores it.
type Problem struct {
	// Type identifies the problem type ("about:blank", the RFC default, when
	// the writer has no registry base).
	Type string `json:"type"`
	// Title is the short summary of the type (the HTTP status phrase).
	Title string `json:"title"`
	// Status repeats the HTTP status code in the body, per the RFC.
	Status int `json:"status"`
	// Detail explains this occurrence.
	Detail string `json:"detail,omitempty"`
	// Instance identifies this occurrence (the request path).
	Instance string `json:"instance,omitempty"`
	// Code is the machine-readable error code (extension).
	Code string `json:"code,omitempty"`
}

// WriteProblem answers with an RFC 9457 problem document. Middleware and entry
// handlers that must answer directly (not through the server error handler)
// use it so every error shares one shape. The type is "about:blank", the
// standard default when no problem registry is configured.
func WriteProblem(c fiber.Ctx, status int, code, detail string) error {
	title := http.StatusText(status)
	if title == "" {
		title = "Request failed"
	}
	// The header goes after JSON: Fiber's JSON sets application/json and would
	// overwrite an earlier value.
	if err := c.Status(status).JSON(Problem{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: c.Path(),
		Code:     code,
	}); err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/problem+json")
	return nil
}

// WriteProblemWith is WriteProblem plus extra extension members (the RFC
// allows them). Used when the error carries structured data, like the failing
// fields of an input validation.
func WriteProblemWith(c fiber.Ctx, status int, code, detail string, extra map[string]any) error {
	title := http.StatusText(status)
	if title == "" {
		title = "Request failed"
	}
	doc := map[string]any{
		"type":     "about:blank",
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": c.Path(),
		"code":     code,
	}
	for k, v := range extra {
		if _, reserved := doc[k]; !reserved {
			doc[k] = v
		}
	}
	// The header goes after JSON: Fiber's JSON sets application/json and would
	// overwrite an earlier value.
	if err := c.Status(status).JSON(doc); err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/problem+json")
	return nil
}

const (
	ErrCodeNotFound     = "ERR_NOT_FOUND"
	ErrCodeValidation   = "ERR_VALIDATION"
	ErrCodeUnauthorized = "ERR_UNAUTHORIZED"
	ErrCodeForbidden    = "ERR_FORBIDDEN"
	ErrCodeRateLimited  = "ERR_RATE_LIMITED"
	ErrCodeConflict     = "ERR_CONFLICT"
	ErrCodeTimeout      = "ERR_TIMEOUT"
	ErrCodeDBConnection = "ERR_DB_CONNECTION"
	ErrCodeDBQuery      = "ERR_DB_QUERY"
	ErrCodeNATS         = "ERR_NATS"
	ErrCodeInternal     = "ERR_INTERNAL"
)

func joinErr(fiberErr *fiber.Error, oopsErr error) error {
	return errors.Join(fiberErr, oopsErr)
}

func errCodeFromStatus(status int) string {
	switch status {
	case fiber.StatusNotFound:
		return ErrCodeNotFound
	case fiber.StatusBadRequest:
		return ErrCodeValidation
	case fiber.StatusUnauthorized:
		return ErrCodeUnauthorized
	case fiber.StatusForbidden:
		return ErrCodeForbidden
	case fiber.StatusTooManyRequests:
		return ErrCodeRateLimited
	case fiber.StatusGatewayTimeout:
		return ErrCodeTimeout
	default:
		return ErrCodeInternal
	}
}

func oopsErr(code, msg string) oops.OopsErrorBuilder {
	return oops.In("runtime").Code(code).Public(msg)
}

func ErrStatus(status int, msg string, kv ...any) error {
	b := oopsErr(errCodeFromStatus(status), msg)
	if len(kv) > 0 {
		b = b.With(kv...)
	}
	return joinErr(fiber.NewError(status, msg), b.Errorf("%s", msg))
}

func ErrNotFound(resource string, id any) error {
	return joinErr(
		fiber.NewError(fiber.StatusNotFound, "resource not found"),
		oopsErr(ErrCodeNotFound, "resource not found").With("resource", resource, "id", id).
			Errorf("%s with id %v not found", resource, id),
	)
}

func ErrDBQuery(op, table string, inner error) error {
	return joinErr(
		fiber.NewError(fiber.StatusInternalServerError, "Database operation failed"),
		oopsErr(ErrCodeDBQuery, "Database operation failed").With("operation", op, "table", table).
			Wrapf(inner, "db %s on %s failed", op, table),
	)
}

func ErrValidation(field, constraint string, val any) error {
	return joinErr(
		fiber.NewError(fiber.StatusBadRequest, "Validation failed"),
		oopsErr(ErrCodeValidation, "Validation failed").With("field", field, "value", val, "constraint", constraint).
			Errorf("validation: %s=%v violates %s", field, val, constraint),
	)
}

func ErrUnauthorized(reason string) error {
	return joinErr(
		fiber.NewError(fiber.StatusUnauthorized, reason),
		oopsErr(ErrCodeUnauthorized, reason).With("reason", reason).
			Errorf("unauthorized: %s", reason),
	)
}

func ErrForbidden(resource string) error {
	return joinErr(
		fiber.NewError(fiber.StatusForbidden, "Access denied"),
		oopsErr(ErrCodeForbidden, "Access denied").With("resource", resource).
			Errorf("forbidden: access to %s denied", resource),
	)
}

func ErrRateLimited(retryAfter int) error {
	return joinErr(
		fiber.NewError(fiber.StatusTooManyRequests, "Too many requests"),
		oopsErr(ErrCodeRateLimited, "Too many requests").With("retry_after", retryAfter).
			Errorf("rate limited, retry after %ds", retryAfter),
	)
}

func ErrTimeout(operation string) error {
	return joinErr(
		fiber.NewError(fiber.StatusGatewayTimeout, "Operation timed out"),
		oopsErr(ErrCodeTimeout, "Operation timed out").With("operation", operation).
			Errorf("timeout: %s", operation),
	)
}

func ErrDBConnection(inner error) error {
	return joinErr(
		fiber.NewError(fiber.StatusInternalServerError, "Database connection failed"),
		oopsErr(ErrCodeDBConnection, "Database connection failed").
			Wrapf(inner, "db connection failed"),
	)
}

func ErrNATSPublish(subject string, inner error) error {
	return joinErr(
		fiber.NewError(fiber.StatusInternalServerError, "Message publishing failed"),
		oopsErr(ErrCodeNATS, "Message publishing failed").With("subject", subject).
			Wrapf(inner, "nats publish to %s failed", subject),
	)
}

func ErrInternal(inner error) error {
	return joinErr(
		fiber.NewError(fiber.StatusInternalServerError, "internal server error"),
		oopsErr(ErrCodeInternal, "internal server error").
			Wrapf(inner, "internal error"),
	)
}
