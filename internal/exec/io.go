package exec

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

type Frame struct {
	Stream   string `json:"stream"`
	Data     string `json:"data,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
}
type lockedEncoder struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

func (e *lockedEncoder) write(f Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.encoder.Encode(f)
}
func CopyFrames(stdout, stderr io.Reader, target io.Writer) error {
	enc := &lockedEncoder{encoder: json.NewEncoder(target)}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	copyStream := func(name string, r io.Reader) {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			if err := enc.write(Frame{Stream: name, Data: s.Text() + "\n"}); err != nil {
				errs <- err
				return
			}
		}
		if err := s.Err(); err != nil {
			errs <- err
		}
		wg.Done()
	}
	wg.Add(2)
	go copyStream("stdout", stdout)
	go copyStream("stderr", stderr)
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}
func Tail(reader io.Reader, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}
	buf := make([]string, lines)
	n := 0
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		buf[n%lines] = s.Text()
		n++
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	start, length := 0, n
	if n > lines {
		start, length = n%lines, lines
	}
	out := ""
	for i := 0; i < length; i++ {
		out += buf[(start+i)%lines] + "\n"
	}
	return out, nil
}
