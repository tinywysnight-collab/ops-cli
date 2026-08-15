package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// promptSession reads interactive answers. Its reads observe ctx so that
// SIGINT — captured by the root signal context, not delivered as a ^C byte by
// a cooked-mode TTY — cancels a pending prompt instead of leaving the command
// blocked on stdin.
type promptSession struct {
	ctx    context.Context
	reader *bufio.Reader
	out    io.Writer
	tty    bool
}

func newPromptSession(ctx context.Context, in io.Reader, out io.Writer, tty bool) *promptSession {
	return &promptSession{ctx: ctx, reader: bufio.NewReader(in), out: out, tty: tty}
}

var interactiveTerminal = func(in io.Reader) bool {
	fd, ok := in.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(fd.Fd()))
}

func (p *promptSession) requireTTY() error {
	if !p.tty {
		return fmt.Errorf("interactive terminal input is required")
	}
	return nil
}

func (p *promptSession) line(prompt string) (string, bool, error) {
	// Cancellation wins even when input is already available: a SIGINT during
	// the prompt must never continue into confirmation or a config write.
	if p.ctx.Err() != nil {
		return "", true, nil
	}
	fmt.Fprint(p.out, prompt)
	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := p.reader.ReadString('\n')
		ch <- readResult{line: line, err: err}
	}()
	select {
	case <-p.ctx.Done():
		return "", true, nil
	case r := <-ch:
		value := strings.TrimSpace(r.line)
		if value == "\x03" {
			return "", true, nil
		}
		if r.err != nil {
			if errors.Is(r.err, io.EOF) {
				if r.line == "" {
					return "", true, nil
				}
				return value, false, nil
			}
			return "", false, fmt.Errorf("read interactive input: %w", r.err)
		}
		return value, false, nil
	}
}

func (p *promptSession) text(label string, optional bool, validate func(string) error) (string, bool, error) {
	for {
		value, cancelled, err := p.line(label + ": ")
		if err != nil || cancelled {
			return "", cancelled, err
		}
		if value == "" && optional {
			return "", false, nil
		}
		if err := validate(value); err != nil {
			fmt.Fprintf(p.out, "  %v\n", err)
			continue
		}
		return value, false, nil
	}
}

func (p *promptSession) selectOne(label string, options []string, active string) (string, bool, error) {
	if len(options) == 0 {
		return "", false, fmt.Errorf("%s has no selectable values", label)
	}
	fmt.Fprintf(p.out, "%s:\n", label)
	for i, option := range options {
		marker := ""
		if option == active {
			marker = " (active)"
		}
		fmt.Fprintf(p.out, "  %d) %s%s\n", i+1, option, marker)
	}
	for {
		raw, cancelled, err := p.line("Select number: ")
		if err != nil || cancelled {
			return "", cancelled, err
		}
		choice, convErr := strconv.Atoi(raw)
		if convErr != nil || choice < 1 || choice > len(options) {
			fmt.Fprintln(p.out, "  invalid selection")
			continue
		}
		return options[choice-1], false, nil
	}
}

func (p *promptSession) confirm() (confirmed, cancelled bool, err error) {
	for {
		value, eofOrInterrupt, err := p.line("Confirm? [y/N]: ")
		if err != nil {
			return false, false, err
		}
		if eofOrInterrupt {
			return false, true, nil
		}
		switch strings.ToLower(value) {
		case "y", "yes":
			return true, false, nil
		case "", "n", "no":
			return false, true, nil
		default:
			fmt.Fprintln(p.out, "  enter y/yes or n/no")
		}
	}
}
