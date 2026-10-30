package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	var res ExecResult
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	conn, err := c.Exec(ctx, namespace, pod, container, command, false)
	if err != nil {
		return res, err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close() // unblocks ReadMessage on timeout
	}()
	for len(stdin) > 0 {
		n := min(len(stdin), 32<<10)
		if err := conn.WriteMessage(wsock.Binary, append([]byte{ExecStdin}, stdin[:n]...)); err != nil {
			return res, err
		}
		stdin = stdin[n:]
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return res, fmt.Errorf("command timed out")
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
