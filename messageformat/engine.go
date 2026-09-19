package messageformat

import (
	"errors"
	"fmt"
	"maps"

	mf2 "github.com/kaptinlin/messageformat-go"
	"github.com/kaptinlin/messageformat-go/pkg/datamodel"
	mferrors "github.com/kaptinlin/messageformat-go/pkg/errors"
	"github.com/kaptinlin/messageformat-go/pkg/functions"
	"golang.org/x/text/language"
)

// This file is the adapter over github.com/kaptinlin/messageformat-go.
// No engine type crosses the package API: messages are converted to and
// from the canonical model, and engine errors are mapped to *Error codes.

// ParseMF2 parses MessageFormat 2 syntax into the canonical model and
// validates it. Syntax errors return an *Error with CodeSyntaxError; valid
// syntax that is not a valid message returns the data model error code.
func ParseMF2(src string) (Message, error) {
	em, err := datamodel.ParseMessage(src)
	if err != nil {
		return Message{}, engineError(err)
	}
	if _, err := datamodel.ValidateMessage(em, nil); err != nil {
		return Message{}, engineError(err)
	}
	msg, err := fromEngine(em)
	if err != nil {
		return Message{}, &Error{Code: CodeInvalidMessage, Message: err.Error()}
	}
	return msg, nil
}

// Validate checks that msg is a valid MF2 message: representable in the
// syntax and free of data model errors.
func Validate(msg Message) error {
	_, err := Stringify(msg)
	return err
}

// Stringify serializes msg as MessageFormat 2 syntax. Options and
// attributes are written in lexicographic order, so the output is stable.
func Stringify(msg Message) (string, error) {
	em, err := toEngineValidated(msg)
	if err != nil {
		return "", err
	}
	src, err := datamodel.StringifyMessage(em)
	if err != nil {
		return "", engineError(err)
	}
	return src, nil
}

// FormatOption configures Format.
type FormatOption func(*formatConfig)

type formatConfig struct {
	bidiIsolation bool
	functions     map[string]functions.MessageFunction
}

// WithBidiIsolation turns bidi isolation of placeholders on (the MF2
// "default" strategy, the default) or off (the "none" strategy). Turn it
// off for output that must not contain isolation characters, such as
// plain-text email subjects or CLI output in left-to-right locales.
func WithBidiIsolation(on bool) FormatOption {
	return func(c *formatConfig) { c.bidiIsolation = on }
}

// withEngineFunctions registers extra engine functions. It is unexported
// on purpose: it exists for the conformance suite's :test:* functions.
func withEngineFunctions(fns map[string]functions.MessageFunction) FormatOption {
	return func(c *formatConfig) { maps.Copy(c.functions, fns) }
}

// Format formats msg for locale (a BCP 47 tag) with the given values.
//
// The required MF2 functions and the draft functions (:currency, :date,
// :datetime, :time, :percent, :unit) are enabled. Formatting follows the
// MF2 error model: the returned string is always usable, with fallback
// representations for placeholders that failed, and the error (a
// *FormatError) lists what failed. A message that is not valid, or an
// invalid locale, returns an *Error and an empty string.
func Format(msg Message, locale string, values map[string]any, opts ...FormatOption) (string, error) {
	if _, err := language.Parse(locale); err != nil {
		return "", &Error{Code: CodeInvalidLocale, Message: fmt.Sprintf("%q: %v", locale, err)}
	}
	cfg := formatConfig{bidiIsolation: true, functions: standardFunctions()}
	for _, opt := range opts {
		opt(&cfg)
	}
	em, err := toEngineValidated(msg)
	if err != nil {
		return "", err
	}
	bidi := mf2.BidiNone
	if cfg.bidiIsolation {
		bidi = mf2.BidiDefault
	}
	formatter, err := mf2.Compile([]string{locale}, em, mf2.WithBidiIsolation(bidi), mf2.WithFunctions(cfg.functions))
	if err != nil {
		return "", engineError(err)
	}
	out, err := formatter.Format(values)
	if err != nil {
		return out, formatErrors(err)
	}
	return out, nil
}

// standardFunctions returns the MF2 required and draft functions.
func standardFunctions() map[string]functions.MessageFunction {
	fns := mf2.DefaultFunctionMap()
	maps.Copy(fns, mf2.DraftFunctionMap())
	return fns
}

// toEngineValidated converts msg and runs the engine's data model checks.
func toEngineValidated(msg Message) (datamodel.Message, error) {
	em, err := toEngine(msg)
	if err != nil {
		return nil, &Error{Code: CodeInvalidMessage, Message: err.Error()}
	}
	if _, err := datamodel.ValidateMessage(em, nil); err != nil {
		return nil, engineError(err)
	}
	return em, nil
}

// --- error mapping -----------------------------------------------------------

// engineKinds maps engine error kinds that differ from the spec names.
var engineKinds = map[string]ErrorCode{
	mferrors.ErrorTypeKeyMismatch:        CodeVariantKeyMismatch,
	mferrors.ErrorTypeMissingFallback:    CodeMissingFallbackVariant,
	mferrors.ErrorTypeInvalidMessage:     CodeInvalidMessage,
	mferrors.ErrorTypeNotFormattable:     CodeBadOperand,
	mferrors.ErrorTypeNoMatch:            CodeBadSelector,
	mferrors.ErrorTypeEmptyToken:         CodeSyntaxError,
	mferrors.ErrorTypeBadEscape:          CodeSyntaxError,
	mferrors.ErrorTypeBadInputExpression: CodeSyntaxError,
	mferrors.ErrorTypeExtraContent:       CodeSyntaxError,
	mferrors.ErrorTypeParseError:         CodeSyntaxError,
	mferrors.ErrorTypeMissingSyntax:      CodeSyntaxError,
}

// engineError maps one engine error to an *Error.
func engineError(err error) *Error {
	var typed interface{ ErrorType() string }
	if !errors.As(err, &typed) {
		return &Error{Code: CodeInvalidMessage, Message: err.Error()}
	}
	kind := typed.ErrorType()
	code, ok := engineKinds[kind]
	if !ok {
		code = ErrorCode(kind)
	}
	var syntaxErr *mferrors.MessageSyntaxError
	var dataModelErr *mferrors.MessageDataModelError
	if errors.As(err, &syntaxErr) && !errors.As(err, &dataModelErr) && !code.IsDataModelError() {
		code = CodeSyntaxError
	}
	return &Error{Code: code, Message: err.Error()}
}

// formatErrors maps the engine's joined format diagnostics.
func formatErrors(err error) *FormatError {
	var errs []error
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	} else {
		errs = []error{err}
	}
	out := &FormatError{Errors: make([]*Error, 0, len(errs))}
	for _, e := range errs {
		out.Errors = append(out.Errors, engineError(e))
	}
	return out
}
