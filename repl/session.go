package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

var (
	ErrTimeout         = errors.New("timeout waiting for prompt")
	ErrCancelled       = errors.New("cancelled")
	ErrIncompleteInput = errors.New("incomplete input (continuation)")
)

/*
   Prompt state

   NodeMCU REPL is stateful:
   "> "  = ready
   ">> " = continuation expected
*/
type PromptKind int

const (
	PromptUnknown PromptKind = iota
	PromptReady
	PromptContinue
)

type Session struct {
	port serial.Port
	mu   sync.Mutex
}

func Open(portName string, baud int) (*Session, error) {
	mode := &serial.Mode{BaudRate: baud}
	p, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}
	_ = p.SetReadTimeout(200 * time.Millisecond)

	return &Session{port: p}, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port.Close()
}

/*
   Low-level prompt wait

   NodeMCU prompt is NOT line based.
   It appears as a suffix in the byte stream:

     "\r\n> "   ready
     "\r\n>> "  continuation

   We therefore watch the stream and classify the prompt.
*/
func (s *Session) waitPrompt(
	ctx context.Context,
	timeout time.Duration,
) ([]byte, PromptKind, error) {

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	var buf bytes.Buffer
	tmp := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			return buf.Bytes(), PromptUnknown, ErrCancelled
		case <-deadline.C:
			return buf.Bytes(), PromptUnknown, ErrTimeout
		default:
		}

		n, err := s.port.Read(tmp)
		if err != nil || n == 0 {
			continue
		}

		buf.WriteByte(tmp[0])

		// keep buffer bounded
		if buf.Len() > 8192 {
			buf.Next(buf.Len() - 8192)
		}

		b := buf.Bytes()

		switch {
		case bytes.HasSuffix(b, []byte("\r\n> ")),
			bytes.HasSuffix(b, []byte("\n> ")):
			return b, PromptReady, nil

		case bytes.HasSuffix(b, []byte("\r\n>> ")),
			bytes.HasSuffix(b, []byte("\n>> ")):
			return b, PromptContinue, nil
		}
	}
}

// Sync nudges the REPL until a ready prompt appears.
func (s *Session) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(3 * time.Second)
	}

	for tries := 0; tries < 5; tries++ {
		_, _ = s.port.Write([]byte("\n"))

		_, prompt, err := s.waitPrompt(
			ctx,
			time.Until(deadline),
		)
		if err == nil && prompt == PromptReady {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return ErrTimeout
}

// Exec sends a single line into the REPL.
// If Lua requests continuation, ErrIncompleteInput is returned.
func (s *Session) Exec(ctx context.Context, line string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	if _, err := s.port.Write([]byte(line)); err != nil {
		return nil, err
	}

	raw, prompt, err := s.waitPrompt(ctx, 7*time.Second)
	if err != nil {
		return splitOutput(raw), err
	}

	if prompt == PromptContinue {
		return splitOutput(raw), ErrIncompleteInput
	}

	return splitOutput(raw), nil
}

// Converts raw byte stream to clean output lines (without prompt)
func splitOutput(raw []byte) []string {
	s := string(raw)

	// remove trailing prompt
	s = strings.TrimSuffix(s, "\r\n> ")
	s = strings.TrimSuffix(s, "\n> ")
	s = strings.TrimSuffix(s, "\r\n>> ")
	s = strings.TrimSuffix(s, "\n>> ")

	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimRight(l, "\r")
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func (s *Session) RemoteList(ctx context.Context) (map[string]int, error) {
	lua := `for k,v in pairs(file.list()) do print(k.."\t"..v) end`
	out, err := s.Exec(ctx, lua)

	m := map[string]int{}
	for _, line := range out {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		var sz int
		fmt.Sscanf(parts[1], "%d", &sz)
		m[parts[0]] = sz
	}
	return m, err
}

func (s *Session) Remove(ctx context.Context, name string) error {
	safe := strings.ReplaceAll(name, `'`, `\'`)
	_, err := s.Exec(ctx, fmt.Sprintf(`file.remove('%s')`, safe))
	return err
}

func (s *Session) WriteFile(
	ctx context.Context,
	name string,
	content []byte,
	progress func(done, total int),
) error {

	safe := strings.ReplaceAll(name, `'`, `\'`)
	if _, err := s.Exec(ctx, fmt.Sprintf(`f=file.open('%s','w')`, safe)); err != nil {
		return err
	}

	total := len(content)
	const chunk = 96
	done := 0

	for done < total {
		select {
		case <-ctx.Done():
			_, _ = s.Exec(context.Background(), `if f then f:close() end`)
			return ErrCancelled
		default:
		}

		end := done + chunk
		if end > total {
			end = total
		}

		part := content[done:end]
		line := fmt.Sprintf("f:write(%q)", string(part))

		if _, err := s.Exec(ctx, line); err != nil {
			_, _ = s.Exec(context.Background(), `if f then f:close() end`)
			return err
		}

		done = end
		if progress != nil {
			progress(done, total)
		}
	}

	_, err := s.Exec(ctx, `f:close()`)
	return err
}

func (s *Session) Interrupt() error {
	_, err := s.port.Write([]byte{0x03}) // Ctrl+C
	return err
}
