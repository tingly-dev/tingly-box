package stage

import "errors"

// CommittedError marks an error raised after a stage committed side effects,
// such as executing a server-owned tool. Retrying the attempt would repeat
// them, so failover must not retry an error for which
// HasCommittedSideEffects reports true.
type CommittedError struct {
	Err error
}

func (e *CommittedError) Error() string { return e.Err.Error() }

func (e *CommittedError) Unwrap() error { return e.Err }

// WrapCommitted marks err as raised after side effects when committed is
// true, and returns it unchanged otherwise, so retry classification before
// the first side effect is unaffected.
func WrapCommitted(err error, committed bool) error {
	if err == nil || !committed || HasCommittedSideEffects(err) {
		return err
	}
	return &CommittedError{Err: err}
}

// HasCommittedSideEffects reports whether err was raised after a stage
// committed side effects.
func HasCommittedSideEffects(err error) bool {
	var committed *CommittedError
	return errors.As(err, &committed)
}
