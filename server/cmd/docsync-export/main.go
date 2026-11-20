// docsync-export is started by an OpenSSH authorized_keys forced command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chendingplano/deepdoc/server/api/docsync"
	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
	"golang.org/x/sys/unix"
)

func main() {
	os.Exit(runExport())
}

func runExport() int {
	// Other packages may have cached stdout before main. Move the original
	// stream to a close-on-exec descriptor and route all console logs to stderr.
	fd, err := unix.FcntlInt(os.Stdout.Fd(), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		_, _ = io.WriteString(os.Stderr, "could not reserve document export stdout\n")
		return 1
	}
	wire := os.NewFile(uintptr(fd), "document export protocol")
	defer wire.Close()
	if err := unix.Dup2(int(os.Stderr.Fd()), int(os.Stdout.Fd())); err != nil {
		_, _ = io.WriteString(os.Stderr, "could not redirect document export console output\n")
		return 1
	}
	return run(wire)
}

func run(wire io.Writer) int {
	_ = godotenv.Load(".env")
	configPath := flag.String("config", "config.toml", "application configuration path")
	outbox := flag.String("outbox", os.Getenv("DOCSYNC_OUTBOX"), "private document export directory")
	ttl := flag.Duration("outbox-ttl", 7*24*time.Hour, "retention for abandoned exports")
	workRun := flag.Int64("work-run", 0, "internal detached pack worker run ID")
	flag.Parse()

	logger := loggerutil.CreateDefaultLogger("20261120-356")
	defer logger.Close()
	original := os.Getenv("SSH_ORIGINAL_COMMAND")
	if flag.NArg() != 0 || *ttl <= 0 || *workRun < 0 ||
		(*workRun == 0 && original == "") || (*workRun != 0 && original != "") {
		logger.Error("invalid document export invocation")
		return 1
	}
	if *outbox == "" {
		*outbox = filepath.Join(".cache", "docsync-outbox")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = context.WithValue(ctx, ApiTypes.CallFlowKey, "20261120-713")

	sourceCallbacks := func(ctx context.Context) (transfer.Callbacks, error) {
		if err := config.LoadConfig(ctx, logger, *configPath); err != nil {
			return transfer.Callbacks{}, errors.New("could not load document export application configuration")
		}
		pg := ApiTypes.CommonConfig.PGConf
		port, err := strconv.Atoi(os.Getenv("PG_PORT"))
		if err != nil || port < 1 || port > 65535 {
			return transfer.Callbacks{}, errors.New("invalid PG_PORT for document export")
		}
		pg.Port, pg.Host = port, os.Getenv("PG_HOST")
		pg.UserName, pg.Password = os.Getenv("PG_USER_NAME"), os.Getenv("PG_PASSWORD")
		root := os.Getenv("ARTIFACT_DIR")
		if root == "" || pg.Host == "" || pg.UserName == "" {
			return transfer.Callbacks{}, errors.New("document export requires ARTIFACT_DIR, PG_HOST and PG_USER_NAME")
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return transfer.Callbacks{}, errors.New("could not resolve the source artifact directory")
		}
		db, err := databaseutil.NamedPostgresPool(pg, os.Getenv("PG_DB_NAME"))
		if err != nil {
			return transfer.Callbacks{}, errors.New("could not initialize document export database pool")
		}
		return (docsync.Source{DB: db, ArtifactRoot: root}).Callbacks(), nil
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)

	server := transfer.Server{
		Outbox: *outbox, OutboxTTL: *ttl, Logger: logger,
		Callbacks: transfer.Callbacks{
			List: func(ctx context.Context, raw json.RawMessage, w io.Writer) error {
				callbacks, err := sourceCallbacks(ctx)
				if err != nil {
					return err
				}
				return callbacks.List(ctx, raw, w)
			},
			Pack: func(ctx context.Context, request transfer.PackRequest, path string) (transfer.PackResult, error) {
				callbacks, err := sourceCallbacks(ctx)
				if err != nil {
					return transfer.PackResult{}, err
				}
				return callbacks.Pack(ctx, request, path)
			},
		},
		StartWorker: func(runID int64, lock *os.File) error {
			return startWorker(runID, lock, *configPath, *outbox, *ttl)
		},
		RejectedSession: func(err error) bool { return errors.Is(err, auth.ErrNativeSession) },
	}
	var err error
	if *workRun != 0 {
		err = server.Work(ctx, *workRun, os.NewFile(3, "document export worker lock"))
	} else {
		auth.InitKratosClient()
		server.Auth = kratosAuth{auth.GetKratosClient()}
		err = server.Serve(ctx, original, os.Stdin, wire)
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, transfer.ErrSession) {
		logger.Warn("document export authentication denied")
		return transfer.SessionExitCode
	}
	logger.Error("document export command failed", "error", err)
	return 1
}

func startWorker(runID int64, lock *os.File, configPath, outbox string, ttl time.Duration) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(filepath.Join(outbox, strconv.FormatInt(runID, 10), "worker.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	cmd := exec.Command(executable, "--work-run", strconv.FormatInt(runID, 10),
		"--config", configPath, "--outbox", outbox, "--outbox-ttl", ttl.String())
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "SSH_ORIGINAL_COMMAND=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.ExtraFiles = []*os.File{lock}
	cmd.Stdout, cmd.Stderr = output, output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The SSH parent exits immediately. The child keeps its own session,
	// file descriptors and inherited worker lock.
	_ = cmd.Process.Release()
	return nil
}

type kratosAuth struct {
	client *auth.KratosClient
}

func (a kratosAuth) LoginNativePassword(ctx context.Context, phone, password string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return a.client.LoginNativePassword(ctx, phone, password)
}

func (a kratosAuth) ValidateNativeSession(ctx context.Context, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return a.client.ValidateNativeSession(ctx, token)
}

func (a kratosAuth) LogoutNativeSession(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return a.client.LogoutNativeSession(ctx, token)
}
