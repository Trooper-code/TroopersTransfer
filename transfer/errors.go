package transfer

import (
	"context"
	"errors"
	"strings"

	"github.com/schollz/croc/v11/src/croc"
)

var (
	ErrBusy     = errors.New("another transfer is already running")
	ErrDeclined = errors.New("transfer declined")
	ErrCanceled = errors.New("transfer canceled")
)

// FriendlyError carries a plain-English message for display alongside the
// original error for logs and "details".
type FriendlyError struct {
	Message string
	Err     error
}

func (e *FriendlyError) Error() string { return e.Message }
func (e *FriendlyError) Unwrap() error { return e.Err }

// Details returns the underlying technical error text.
func (e *FriendlyError) Details() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// friendly converts croc's internal errors into messages a non-technical
// user can act on. isSender changes the wording for errors that depend on
// which side saw them.
func friendly(err error, isSender bool) error {
	if err == nil {
		return nil
	}
	var fe *FriendlyError
	if errors.As(err, &fe) {
		return err
	}
	msg := strings.ToLower(err.Error())
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(msg, s) {
				return true
			}
		}
		return false
	}
	wrap := func(m string) error { return &FriendlyError{Message: m, Err: err} }

	switch {
	case errors.Is(err, ErrBusy):
		return wrap("Another transfer is already running. Wait for it to finish or cancel it first.")
	case errors.Is(err, ErrCanceled), errors.Is(err, context.Canceled):
		return wrap("Transfer canceled.")
	case errors.Is(err, ErrDeclined):
		if isSender {
			return wrap("The receiver declined the files.")
		}
		return wrap("You declined the files. Nothing was saved.")
	case has("refused files", "refusing files"):
		if isSender {
			return wrap("The receiver declined the files.")
		}
		return wrap("You declined the files. Nothing was saved.")
	case has("code is too short"):
		return wrap("The code phrase must be at least 6 characters.")
	case has("bad password", "relay admission rejected"):
		return wrap("The relay rejected the password. Check the relay password in Settings.")
	case has("password mismatch", "pake", "could not secure channel", "incorrect passphrase", "message authentication failed", "cipher: message authentication"):
		return wrap("Wrong code. Double-check the code phrase and try again.")
	case has("room (secure channel) not ready", "room not ready"):
		if isSender {
			return wrap("The receiver disconnected before the transfer started.")
		}
		return wrap("No one is sending with that code. Check the code, or the sender may have closed the app.")
	case has("relay room full"):
		return wrap("That code is already in use by another transfer. Try a different code.")
	case has("rate limited"):
		return wrap("The relay is busy. Wait a moment and try again.")
	case has("not enough disk space", "no space left", "disk full"):
		return wrap("Not enough disk space in the destination folder.")
	case has("received archive failed validation"):
		return wrap("The received folder archive was damaged or unsafe, so it was not unpacked.")
	case has("must be a local path", "escapes receive directory", "unsafe path", "outside the destination"):
		return wrap("The sender offered a file with an unsafe name. The transfer was blocked for your safety.")
	case has("peer error"):
		if isSender {
			return wrap("The receiver stopped the transfer.")
		}
		return wrap("Sender closed the app or canceled the transfer.")
	case has("permission denied", "access is denied"):
		return wrap("Couldn't write to the destination folder. Choose a different folder.")
	case errors.Is(err, croc.ErrRelayConnection), errors.Is(err, croc.ErrDERPConnection),
		has("could not connect", "no such host", "connection refused", "i/o timeout", "could not reconnect to any relay"):
		return wrap("Couldn't reach the relay server. Check your internet connection or the relay address in Settings.")
	case has("transfer disconnected", "eof", "connection reset", "broken pipe", "forcibly closed", "no pong", "use of closed network connection"):
		return wrap("Connection lost, try again.")
	}
	return wrap("Something went wrong: " + err.Error())
}
