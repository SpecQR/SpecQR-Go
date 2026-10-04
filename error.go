package specqr

// ErrorCode is a stable, language-neutral error category.
type ErrorCode string

const (
	InvalidInput            ErrorCode = "INVALID_INPUT"
	InvalidMode             ErrorCode = "INVALID_MODE"
	InvalidVersion          ErrorCode = "INVALID_VERSION"
	InvalidECC              ErrorCode = "INVALID_ECC"
	InvalidECI              ErrorCode = "INVALID_ECI"
	InvalidGS1              ErrorCode = "INVALID_GS1"
	InvalidStructuredAppend ErrorCode = "INVALID_STRUCTURED_APPEND"
	InvalidColor            ErrorCode = "INVALID_COLOR"
	DataTooLong             ErrorCode = "DATA_TOO_LONG"
	ResourceLimit           ErrorCode = "RESOURCE_LIMIT"
)

// Error reports malformed input and resource constraints without panicking.
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string            { return string(e.Code) + ": " + e.Message }
func errCode(c ErrorCode, m string) error { return &Error{c, m} }
