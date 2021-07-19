package wfapi

import (
	"encoding/json"
	"fmt"
	"os"
)

type Error struct {
	Code    string      // Something you should be reasonably able to switch upon programmatically in an API.  Sometimes blank, meaning we've wrapped an unknown error, and the Message string is all you've got.
	Message string      // Complete, preformatted message.  Often duplicates some content that may also be found in the Details.
	Details [][2]string // Key:Value ordered details.  Serializes as map.
	Cause   *Error      // Your option to recurse.  Is `*Error` and not `error` because we still want this to have a predictable, explicit json structure (and be unmarshallable).
}

func (e *Error) Error() string {
	return e.Message
}

// wrap takes an unknown error, and if it's *Error, returns it as such;
// if it's any other golang error, it wraps it in an *Error which has only the message field set.
//
// This may lose type information (e.g. it's not friendly to `errors.Is`);
// that's a trade we make, because we care about the value being equal to what it will be after a serialization roundtrip.
func wrapErr(err error) *Error {
	switch c2 := err.(type) {
	case *Error:
		return c2
	default:
		return &Error{
			Message: err.Error(),
		}
	}
}

// TerminalError emits an error on stdout as json, and halts immediately.
// In most cases, you should not use this method, and there will be a better place to send errors
// that will be more guaranteed to fit any protocols and scripts better;
// however, this is sometimes used in init methods (where we know no other protocol yet).
func TerminalError(err *Error, exitCode int) {
	json.NewEncoder(os.Stdout).Encode(struct {
		Error *Error `json:"error"`
	}{err})
	os.Exit(exitCode)
}

func ErrorUnknown(msgTmpl string, cause error) *Error {
	return &Error{
		Code:    "warpforge-error-unknown",
		Message: msgTmpl,
		Cause:   wrapErr(cause),
	}
}

func ErrorSearchingFilesystem(searchingFor string, cause error) *Error {
	return &Error{
		Code:    "warpforge-error-searching-filesystem",
		Message: fmt.Sprintf("error while searching filesystem for %s: %s", searchingFor, cause),
		Details: [][2]string{
			{"searchingFor", searchingFor},
			// the cause is presumed to have any path(s) relevant.
		},
		Cause: wrapErr(cause),
	}
}

func ErrorWorkspace(wsPath string, cause error) *Error {
	return &Error{
		Code:    "warpforge-error-workspace",
		Message: fmt.Sprintf("error handling workspace at %q: %s", wsPath, cause),
		Details: [][2]string{
			{"workspacePath", wsPath},
		},
		Cause: wrapErr(cause),
	}
}

func ErrorExecutorFailed(executorEngineName string, cause error) *Error {
	return &Error{
		Code:    "warpforge-error-executor-failed",
		Message: fmt.Sprintf("executor engine failed: the %s engine reported error: %s", executorEngineName, cause),
		Details: [][2]string{
			{"engineName", executorEngineName},
			// ideally we'd have more details here, but honestly, our executors don't give us much clarity most of the time, so... we'll see.
		},
		Cause: wrapErr(cause),
	}
}