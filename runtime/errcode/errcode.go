package errcode

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/samber/oops"
)

const (
	ErrCodeNotFound     = "ERR_NOT_FOUND"
	ErrCodeValidation   = "ERR_VALIDATION"
	ErrCodeUnauthorized = "ERR_UNAUTHORIZED"
	ErrCodeForbidden    = "ERR_FORBIDDEN"
	ErrCodeRateLimited  = "ERR_RATE_LIMITED"
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
