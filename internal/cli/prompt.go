package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
)

type promptSession struct {
	reader *bufio.Reader
	out    io.Writer
	tty    bool
}

func newPromptSession(in io.Reader, out io.Writer, tty bool) *promptSession {
	return &promptSession{reader: bufio.NewReader(in), out: out, tty: tty}
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
	fmt.Fprint(p.out, prompt)
	line, err := p.reader.ReadString('\n')
	value := strings.TrimSpace(line)
	if value == "\x03" {
		return "", true, nil
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			if line == "" {
				return "", true, nil
			}
			return value, false, nil
		}
		return "", false, fmt.Errorf("read interactive input: %w", err)
	}
	return value, false, nil
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
