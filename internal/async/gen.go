//go:build ignore
// +build ignore

package main

// To support a new type in this package, one can add types to `codes`,
// then run: `go generate ./...` in this folder, to generate more desired
// concrete typed unbounded channels or queues.
//
// Note that chan_struct.go is a specialized implementation for struct{}
// objects. If one changes the code template, then those changes should
// also be synced to chan_struct.go file manually.

import (
	"bytes"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"text/template"
)

type data struct {
	Type    string
	Name    string
	Imports string
}

func main() {
	codes := map[*template.Template]map[string]data{
		chanImpl: {
			"chan_canvasobject.go": {
				Type:    "fyne.CanvasObject",
				Name:    "CanvasObject",
				Imports: `import "fyne.io/fyne/v2"`,
			},
			"chan_func.go": {
				Type:    "func()",
				Name:    "Func",
				Imports: "",
			},
			"chan_interface.go": {
				Type:    "interface{}",
				Name:    "Interface",
				Imports: "",
			},
		},
	}

	for tmpl, types := range codes {
		for fname, data := range types {
			buf := &bytes.Buffer{}
			err := tmpl.Execute(buf, data)
			if err != nil {
				panic(fmt.Errorf("failed to generate %s for type %s: %v", tmpl.Name(), data.Type, err))
			}

			code, err := format.Source(buf.Bytes())
			if err != nil {
				panic(fmt.Errorf("failed to format the generated code:\n%v", err))
			}

			os.WriteFile(fname, code, fs.ModePerm)
		}
	}
}

var chanImpl = template.Must(template.New("async").Parse(`// Code generated by go run gen.go; DO NOT EDIT.
//go:build !go1.21
package async

{{.Imports}}

// Unbounded{{.Name}}Chan is a channel with an unbounded buffer for caching
// {{.Name}} objects. A channel must be closed via Close method.
type Unbounded{{.Name}}Chan struct {
	in, out chan {{.Type}}
	close   chan struct{}
	q       []{{.Type}}
}

// NewUnbounded{{.Name}}Chan returns a unbounded channel with unlimited capacity.
func NewUnbounded{{.Name}}Chan() *Unbounded{{.Name}}Chan {
	ch := &Unbounded{{.Name}}Chan{
		// The size of {{.Name}} is less than 16 bytes, we use 16 to fit
		// a CPU cache line (L2, 256 Bytes), which may reduce cache misses.
		in:  make(chan {{.Type}}, 16),
		out: make(chan {{.Type}}, 16),
		close: make(chan struct{}),
	}
	go ch.processing()
	return ch
}

// In returns the send channel of the given channel, which can be used to
// send values to the channel.
func (ch *Unbounded{{.Name}}Chan) In() chan<- {{.Type}} { return ch.in }

// Out returns the receive channel of the given channel, which can be used
// to receive values from the channel.
func (ch *Unbounded{{.Name}}Chan) Out() <-chan {{.Type}} { return ch.out }

// Close closes the channel.
func (ch *Unbounded{{.Name}}Chan) Close() { ch.close <- struct{}{} }

func (ch *Unbounded{{.Name}}Chan) processing() {
	// This is a preallocation of the internal unbounded buffer.
	// The size is randomly picked. But if one changes the size, the
	// reallocation size at the subsequent for loop should also be
	// changed too. Furthermore, there is no memory leak since the
	// queue is garbage collected.
	ch.q = make([]{{.Type}}, 0, 1<<10)
	for {
		select {
		case e, ok := <-ch.in:
			if !ok {
				// We don't want the input channel be accidentally closed
				// via close() instead of Close(). If that happens, it is
				// a misuse, do a panic as warning.
				panic("async: misuse of unbounded channel, In() was closed")
			}
			ch.q = append(ch.q, e)
		case <-ch.close:
			ch.closed()
			return
		}
		for len(ch.q) > 0 {
			select {
			case ch.out <- ch.q[0]:
				ch.q[0] = nil // de-reference earlier to help GC
				ch.q = ch.q[1:]
			case e, ok := <-ch.in:
				if !ok {
					// We don't want the input channel be accidentally closed
					// via close() instead of Close(). If that happens, it is
					// a misuse, do a panic as warning.
					panic("async: misuse of unbounded channel, In() was closed")
				}
				ch.q = append(ch.q, e)
			case <-ch.close:
				ch.closed()
				return
			}
		}
		// If the remaining capacity is too small, we prefer to
		// reallocate the entire buffer.
		if cap(ch.q) < 1<<5 {
			ch.q = make([]{{.Type}}, 0, 1<<10)
		}
	}
}

func (ch *Unbounded{{.Name}}Chan) closed() {
	close(ch.in)
	for e := range ch.in {
		ch.q = append(ch.q, e)
	}
	for len(ch.q) > 0 {
		select {
		case ch.out <- ch.q[0]:
			ch.q[0] = nil // de-reference earlier to help GC
			ch.q = ch.q[1:]
		default:
		}
	}
	close(ch.out)
	close(ch.close)
}
`))
