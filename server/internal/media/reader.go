package media

import (
	"errors"
	"io"
)

// errTooLarge is the sentinel a reader stops with when the bytes exceed
// what that kind of file is allowed. It travels up through whatever was
// copying, which is why the copier's error is wrapped rather than replaced.
var errTooLarge = errors.New("media: file is too large")

// guardedReader passes bytes through and counts them, and stops with
// errTooLarge rather than truncating.
//
// Truncating would store a broken file and call it a success, which is the
// worst of the options: the sender is told it worked and the receiver gets
// half a video.
type guardedReader struct {
	r         io.Reader
	remaining int64
	read      int64
}

func (g *guardedReader) Read(p []byte) (int, error) {
	if g.remaining <= 0 {
		return 0, errTooLarge
	}
	if int64(len(p)) > g.remaining+1 {
		p = p[:g.remaining+1]
	}
	n, err := g.r.Read(p)
	g.read += int64(n)
	g.remaining -= int64(n)
	if g.remaining < 0 {
		return n, errTooLarge
	}
	return n, err
}

// readLimited reads everything, up to a limit, and refuses more.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	g := &guardedReader{r: r, remaining: limit}
	data, err := io.ReadAll(g)
	if err != nil {
		return nil, err
	}
	return data, nil
}
