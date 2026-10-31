package transfer

import (
	"bufio"
	"context"
	"io"
	"sync"
	_ "unsafe" // for go:linkname

	_ "github.com/schollz/croc/v11/src/utils" // ensure the linknamed symbol is linked
)

// croc asks "Accept ... (Y/n)?" by reading a line from a package-level
// bufio.Reader wrapped around os.Stdin. A GUI process has no usable stdin, so
// we swap that reader for one backed by promptReader. When croc tries to read,
// we know it is waiting for a decision and can ask the user instead.
//
//go:linkname crocStdinReader github.com/schollz/croc/v11/src/utils.stdinReader
var crocStdinReader *bufio.Reader

// promptReader answers croc's stdin prompts. The first read is the accept
// prompt and is answered by calling ask. Every later read is a "y/N" overwrite
// prompt, which we always decline (we receive into an empty staging folder,
// so these should never appear).
type promptReader struct {
	ctx   context.Context
	ask   func() bool
	mu    sync.Mutex
	asked bool
	buf   []byte
}

func (p *promptReader) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.buf) == 0 {
		if err := p.ctx.Err(); err != nil {
			return 0, io.EOF
		}
		answer := "n\n"
		if !p.asked {
			p.asked = true
			if p.ask() {
				answer = "y\n"
			}
		}
		p.buf = []byte(answer)
	}
	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func installPrompt(p *promptReader) {
	crocStdinReader = bufio.NewReader(p)
}
