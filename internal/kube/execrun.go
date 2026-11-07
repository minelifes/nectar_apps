package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"nectar_apps/internal/wsock"
)

// ExecResult is the outcome of a non-interactive command.
type ExecResult struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
}

// ErrOutputTooLarge: the command wrote more than the allowed output.
var ErrOutputTooLarge = errors.New("output too large")

// ExecRun runs command in a container without a terminal, writes stdin (if any) and collects the output
// up to maxOut bytes. Commands that read stdin must stop by themselves (e.g. "head -c N"): the
// v4 exec protocol cannot close stdin. A non-zero exit is not an error (see ExitCode).
func (c *Client) ExecRun(ctx context.Context, namespace, pod, container string, command []string, stdin []byte, maxOut int) (ExecResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var in io.Reader
	if len(stdin) > 0 {
		in = bytes.NewReader(stdin)
	}
	return c.ExecStream(ctx, namespace, pod, container, command, in, maxOut)
}

// ExecStream is ExecRun with stdin read from a stream (nil = none) and no time limit of its own (ctx
// decides): for long transfers such as unpacking a large archive. Stdin is sent while the output is
// read, so a command that fails early is noticed at once. The same rule about stdin applies: the
// command must stop reading by itself.
func (c *Client) ExecStream(ctx context.Context, namespace, pod, container string, command []string, stdin io.Reader, maxOut int) (ExecResult, error) {
	var res ExecResult
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := c.Exec(ctx, namespace, pod, container, command, false)
	if err != nil {
		return res, err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close() // unblocks ReadMessage on cancel / timeout
	}()
	writeErr := make(chan error, 1)
	if stdin != nil {
		go func() {
			buf := make([]byte, 1+32<<10)
			buf[0] = ExecStdin
			for {
				n, err := stdin.Read(buf[1:])
				if n > 0 {
					if werr := conn.WriteMessage(wsock.Binary, buf[:1+n]); werr != nil {
						writeErr <- werr
						return
					}
				}
				if errors.Is(err, io.EOF) {
					return
				}
				if err != nil {
					writeErr <- err
					cancel()
					return
				}
			}
		}()
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			select {
			case werr := <-writeErr:
				return res, fmt.Errorf("sending input: %w", werr)
			default:
			}
			if ctx.Err() != nil {
				return res, fmt.Errorf("command timed out or was cancelled")
			}
			return res, err
		}
		if len(msg) == 0 {
			continue
		}
		switch msg[0] {
		case ExecStdout:
			if out.Len()+len(msg)-1 > maxOut {
				return res, ErrOutputTooLarge
			}
			out.Write(msg[1:])
		case ExecStderr:
			if errOut.Len() < 8<<10 {
				errOut.Write(msg[1:])
			}
		case ExecError:
			res.Stdout, res.Stderr = out.Bytes(), errOut.String()
			res.ExitCode = exitCode(msg[1:])
			return res, nil
		}
	}
}

// exitCode reads the exit code from the exec status (0 for Success).
func exitCode(status []byte) int {
	var st struct {
		Status  string `json:"status"`
		Details struct {
			Causes []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"causes"`
		} `json:"details"`
	}
	if json.Unmarshal(status, &st) != nil || st.Status == "Success" {
		return 0
	}
	for _, c := range st.Details.Causes {
		if c.Reason == "ExitCode" {
			if n, err := strconv.Atoi(c.Message); err == nil {
				return n
			}
		}
	}
	return 1
}
