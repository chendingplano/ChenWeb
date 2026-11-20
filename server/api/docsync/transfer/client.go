package transfer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"golang.org/x/time/rate"
)

type SSHConfig struct {
	Host              string
	User              string
	Port              int
	MaxBytesPerSecond int64
	Logger            ApiTypes.JimoLogger
	OnRetry           func(context.Context, int64, int, int64, error)
}

type Client struct {
	mu      sync.Mutex
	dir     string
	sshArgs []string
	token   string
	login   loginRequest
	logger  ApiTypes.JimoLogger
	limiter *rate.Limiter
	onRetry func(context.Context, int64, int, int64, error)
}

var (
	sshHost = regexp.MustCompile(`^[[:alnum:]:][[:alnum:]:._%-]*$`)
	sshUser = regexp.MustCompile(`^[[:alnum:]_][[:alnum:]_.-]*$`)
)

// OpenSSH uses the caller's SSH configuration, keys and known_hosts.
func OpenSSH(config SSHConfig) (*Client, error) {
	if !sshHost.MatchString(config.Host) || !sshUser.MatchString(config.User) ||
		config.Port < 1 || config.Port > 65535 || config.MaxBytesPerSecond < 0 {
		return nil, errors.New("invalid document sync SSH configuration")
	}
	// Keep the control socket path short enough for macOS and Linux.
	dir, err := os.MkdirTemp("/tmp", "docsync-ssh-")
	if err != nil {
		return nil, err
	}
	var limiter *rate.Limiter
	if config.MaxBytesPerSecond > 0 {
		limiter = rate.NewLimiter(rate.Limit(config.MaxBytesPerSecond), int(min(config.MaxBytesPerSecond, 32*1024)))
	}
	return &Client{
		dir: dir, logger: config.Logger, onRetry: config.OnRetry, limiter: limiter,
		sshArgs: []string{
			"-T", "-p", strconv.Itoa(config.Port), "-l", config.User,
			"-o", "BatchMode=yes",
			"-o", "StrictHostKeyChecking=yes",
			"-o", "PasswordAuthentication=no",
			"-o", "KbdInteractiveAuthentication=no",
			"-o", "PreferredAuthentications=publickey",
			"-o", "ClearAllForwardings=yes",
			"-o", "ForwardAgent=no",
			"-o", "ForwardX11=no",
			"-o", "PermitLocalCommand=no",
			"-o", "ConnectTimeout=30",
			"-o", "ServerAliveInterval=20",
			"-o", "ServerAliveCountMax=3",
			"-o", "ControlMaster=auto",
			"-o", "ControlPersist=120",
			"-S", filepath.Join(dir, "%C"),
			"--", config.Host,
		},
	}, nil
}

func (c *Client) Login(ctx context.Context, phone, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.login = loginRequest{Phone: phone, Password: password}
	return c.authenticate(ctx)
}

func (c *Client) authenticate(ctx context.Context) error {
	c.token = ""
	if c.login.Phone == "" || c.login.Password == "" {
		return ErrSession
	}
	var response bytes.Buffer
	if err := c.execute(ctx, "export login", c.login, &response); err != nil {
		return err
	}
	var reply loginReply
	if err := readJSON(&response, &reply); err != nil || reply.Token == "" {
		return errors.New("invalid document sync login response")
	}
	c.token = reply.Token
	return nil
}

func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		c.token = ""
		c.login = loginRequest{}
	}()
	if c.token == "" {
		return nil
	}
	err := c.execute(ctx, "export logout", envelope{Token: c.token}, io.Discard)
	if errors.Is(err, ErrSession) {
		return nil
	}
	return err
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	c.login = loginRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := append([]string{"-O", "exit"}, c.sshArgs...)
	_ = exec.CommandContext(ctx, "ssh", args...).Run()
	return os.RemoveAll(c.dir)
}

func (c *Client) List(ctx context.Context, request any, result io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	done := make(chan error, 1)
	go func() {
		err := c.call(ctx, "export list", request, writer)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	z, err := gzip.NewReader(reader)
	if err == nil {
		_, err = io.Copy(result, z)
		_ = z.Close()
	}
	if err != nil {
		cancel()
		_ = reader.Close()
	}
	return errors.Join(err, <-done)
}

func (c *Client) Pack(ctx context.Context, request PackRequest) error {
	return c.call(ctx, fmt.Sprintf("export pack --run %d --tar %d", request.RunID, request.TarNumber), request, io.Discard)
}

func (c *Client) Status(ctx context.Context, runID int64) (Index, error) {
	var buf bytes.Buffer
	if err := c.call(ctx, fmt.Sprintf("export status --run %d", runID), nil, &buf); err != nil {
		return Index{}, err
	}
	var result Index
	if err := readJSON(&buf, &result); err != nil {
		return Index{}, err
	}
	if result.RunID != runID {
		return Index{}, errors.New("document export status run ID mismatch")
	}
	return result, nil
}

func (c *Client) Ack(ctx context.Context, runID int64, name string) error {
	command := fmt.Sprintf("export ack --run %d", runID)
	if name != "" {
		command += " --tar " + name
		parsed, err := parseCommand(command)
		if err != nil || parsed.Name != name {
			return errCommand
		}
	}
	return c.call(ctx, command, nil, io.Discard)
}

func (c *Client) call(ctx context.Context, command string, request any, result io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := envelope{Token: c.token}
	if request != nil {
		body, err := json.Marshal(request)
		if err != nil {
			return errors.New("could not encode document export request")
		}
		msg.Body = body
	}
	if msg.Token == "" {
		if err := c.authenticate(ctx); err != nil {
			return err
		}
		msg.Token = c.token
	}
	err := c.execute(ctx, command, msg, result)
	if !errors.Is(err, ErrSession) {
		return err
	}
	if err := c.authenticate(ctx); err != nil {
		return err
	}
	msg.Token = c.token
	return c.execute(ctx, command, msg, result)
}

func (c *Client) execute(ctx context.Context, input string, request any, result io.Writer) error {
	cmdInfo, err := parseCommand(input)
	if err != nil {
		return err
	}
	// Only canonical fixed command syntax and typed arguments reach the remote shell.
	body, err := json.Marshal(request)
	if err != nil || len(body) > maxRequestSize {
		return errors.New("could not encode document export request within its size limit")
	}
	args := append(append([]string{}, c.sshArgs...), input)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = result
	if cmdInfo.Action == "fetch" && c.limiter != nil {
		cmd.Stdout = &throttleWriter{ctx: ctx, out: result, limiter: c.limiter}
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case SessionExitCode:
				return ErrSession
			case 255:
				return fmt.Errorf("document export %s: %w", cmdInfo.Action, ErrSSHConnection)
			}
		}
		return fmt.Errorf("document export %s over SSH failed: %w", cmdInfo.Action, err)
	}
	return nil
}

type throttleWriter struct {
	ctx     context.Context
	out     io.Writer
	limiter *rate.Limiter
}

func (w *throttleWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := min(len(p), w.limiter.Burst())
		if err := w.limiter.WaitN(w.ctx, chunk); err != nil {
			return written, err
		}
		n, err := w.out.Write(p[:chunk])
		written += n
		if err != nil {
			return written, err
		}
		if n != chunk {
			return written, io.ErrShortWrite
		}
		p = p[n:]
	}
	return written, nil
}
