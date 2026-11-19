// Command release-tool manages release versions from scripts and agents (openspec
// release-versioning). It runs from the ChenWeb root.
//
//	release-tool reserve --pid <build script pid>        start a build's release; prints RELEASE_VERSION=<v>
//	release-tool notes --version <v> --build-number <n> --out <dir> [--description <d>]
//	                                                    record the build, write release.json and the notes
//	release-tool commit --version <v>                    release it; prints ::release::<v>
//	release-tool abort --version <v>                     drop a building release; its items return to pending
//	release-tool add-item --type <t> --component <c> [--issue <n>] [--pr <p>] [--notes <x>] <description>
//	release-tool import <release.json> [--set-current]   add a release file's history (deploy machine)
//
// Every command except import runs only where [deployment] role = "build". Exit codes:
// 0 success, 1 failure, 2 usage error.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chendingplano/deepdoc/server/api/releaseshandler"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

const usageText = `usage:
  release-tool reserve --pid <pid>
  release-tool notes --version <v> --build-number <n> --out <dir> [--description <d>]
  release-tool commit --version <v>
  release-tool abort --version <v>
  release-tool add-item --type "bug fix|improvement|new feature" --component ChenWeb|shared [--issue <n>] [--pr <p>] [--notes <x>] <description>
  release-tool import <release.json> [--set-current]
common flags: --project ChenWeb  --config ./config.toml`

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageText)
		return 2
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", releaseshandler.ProjectChenWeb, "project")
	configPath := fs.String("config", "./config.toml", "path to config.toml")
	version := fs.String("version", "", "release version MM.mm.rr")
	pid := fs.Int("pid", 0, "pid of the build script")
	buildNumber := fs.String("build-number", "", "build number")
	out := fs.String("out", "", "directory for release.json and the notes")
	description := fs.String("description", "", "build description")
	itemType := fs.String("type", "", "item type")
	component := fs.String("component", "", "item component")
	issue := fs.String("issue", "", "GitHub issue number")
	pr := fs.String("pr", "", "pull request")
	notes := fs.String("notes", "", "item notes")
	setCurrent := fs.Bool("set-current", false, "import: make the file's release this machine's current release")
	if err := fs.Parse(reorder(args[1:])); err != nil {
		return 2
	}
	rest := fs.Args()

	var v releaseshandler.Version
	needVersion := cmd == "notes" || cmd == "commit" || cmd == "abort"
	if needVersion {
		var err error
		if v, err = releaseshandler.ParseVersion(*version); err != nil {
			fmt.Fprintln(stderr, "release-tool:", err)
			return 2
		}
	}
	switch {
	case cmd == "reserve" && *pid <= 0,
		cmd == "notes" && (*buildNumber == "" || *out == ""),
		cmd == "add-item" && len(rest) != 1,
		cmd == "import" && len(rest) != 1:
		fmt.Fprintln(stderr, usageText)
		return 2
	case cmd != "reserve" && cmd != "notes" && cmd != "commit" && cmd != "abort" && cmd != "add-item" && cmd != "import":
		fmt.Fprintln(stderr, usageText)
		return 2
	}

	logger := loggerutil.CreateDefaultLogger("20261119-201")
	defer logger.Close()
	ctx := context.WithValue(context.Background(), ApiTypes.CallFlowKey, "20261119-202")
	_ = godotenv.Load("./.env")
	ApiUtils.LoadLibConfig("20261119-203")
	if err := config.LoadConfig(ctx, logger, *configPath); err != nil {
		fmt.Fprintln(stderr, "release-tool: load config:", err)
		return 1
	}
	if cmd != "import" && strings.TrimSpace(config.AppConfig.Deployment.Role) != releaseshandler.RoleBuild {
		fmt.Fprintln(stderr, `release-tool: releases are changed only on the build machine ([deployment] role = "build")`)
		return 1
	}
	if err := databaseutil.InitDB(ctx, ApiTypes.CommonConfig); err != nil {
		fmt.Fprintln(stderr, "release-tool: init database:", err)
		return 1
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)
	db := ApiTypes.ProjectDBHandle

	var err error
	switch cmd {
	case "reserve":
		var r releaseshandler.Release
		host, _ := os.Hostname()
		r, err = releaseshandler.Reserve(ctx, db, *project, host, *pid, pidAlive, whoami())
		if err == nil {
			logger.Info("release-tool: reserved", "version", r.Version, "pid", *pid, "host", host)
			fmt.Fprintf(stdout, "RELEASE_VERSION=%s\n", r.Version)
		}
	case "notes":
		err = writeNotes(ctx, *project, v, releaseshandler.BuildInfo{BuildNumber: *buildNumber, Description: *description}, *out, stdout)
		if err == nil {
			logger.Info("release-tool: notes written", "version", v.String(), "build_number", *buildNumber, "out", *out)
		}
	case "commit":
		if err = releaseshandler.Commit(ctx, db, *project, v); err == nil {
			logger.Info("release-tool: committed", "version", v.String())
			fmt.Fprintf(stdout, "::release::%s\n", v)
		}
	case "abort":
		if err = releaseshandler.Abort(ctx, db, *project, v); err == nil {
			logger.Info("release-tool: aborted", "version", v.String())
			fmt.Fprintf(stdout, "release %s given back; its items are pending again\n", v)
		}
	case "add-item":
		var it releaseshandler.Item
		it, err = releaseshandler.AddItem(ctx, db, *project, releaseshandler.Item{ItemType: *itemType, Component: *component,
			Description: rest[0], Issue: *issue, PullRequest: *pr, Notes: *notes}, whoami())
		if err == nil {
			logger.Info("release-tool: item added", "id", it.ID, "type", it.ItemType)
			fmt.Fprintf(stdout, "added pending item %d: %s\n", it.ID, it.Description)
		}
	case "import":
		err = importFile(ctx, rest[0], *setCurrent, stdout)
	}
	if err != nil {
		logger.Error("release-tool: failed", "command", cmd, "error", err)
		fmt.Fprintln(stderr, "release-tool:", err)
		if errors.Is(err, releaseshandler.ErrInvalid) {
			return 2
		}
		return 1
	}
	return 0
}

// reorder moves flags before positional arguments, so `add-item "text" --type x` parses.
func reorder(args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			pos = append(pos, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && a != "--set-current" && a != "-set-current" && i+1 < len(args):
			flags = append(flags, a, args[i+1])
			i++
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
		default:
			pos = append(pos, a)
		}
	}
	return append(flags, pos...)
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func whoami() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host, _ := os.Hostname()
	return name + "@" + host
}

// gitState returns the commit and dirty flag of the repository at dir ("" when it is not one).
func gitState(dir string) (string, bool) {
	rev, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	status, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return strings.TrimSpace(string(rev)), len(strings.TrimSpace(string(status))) > 0
}

func writeNotes(ctx context.Context, project string, v releaseshandler.Version, info releaseshandler.BuildInfo, out string, stdout io.Writer) error {
	db := ApiTypes.ProjectDBHandle
	info.GitRev, info.GitDirty = gitState(".")
	info.SharedGitRev, info.SharedGitDirty = gitState("../shared")
	if err := releaseshandler.SetBuildInfo(ctx, db, project, v, info); err != nil {
		return err
	}
	f, err := releaseshandler.BuildFile(ctx, db, project, v, time.Now())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := releaseshandler.WriteFile(filepath.Join(out, "release.json"), f); err != nil {
		return err
	}
	from := releaseshandler.PreviousBuild(f.History, v)
	releases := releaseshandler.InRange(f.History, from, v)
	pdf := releaseshandler.NewPDFRenderer(".cache/release-notes")
	for _, lang := range releaseshandler.Langs {
		n := releaseshandler.Render(project, releases, from, v, lang)
		base := "RELEASE_NOTES"
		if lang != "en" {
			base += "." + lang
		}
		if err := os.WriteFile(filepath.Join(out, base+".txt"), []byte(n.Text), 0o644); err != nil {
			return err
		}
		path, err := pdf.PDF(ctx, n.Typst)
		if err != nil {
			return fmt.Errorf("%s PDF: %w", lang, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, base+".pdf"), data, 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "release %s: %d item(s); notes in %s\n", v, len(f.History[0].Items), out)
	return nil
}

func importFile(ctx context.Context, path string, setCurrent bool, stdout io.Writer) error {
	f, err := releaseshandler.ReadFile(path)
	if err != nil {
		return err
	}
	db := ApiTypes.ProjectDBHandle
	added, err := releaseshandler.Import(ctx, db, f)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "imported %d new release(s) from %s\n", added, path)
	if setCurrent {
		v, _ := releaseshandler.ParseVersion(f.Version)
		if err := releaseshandler.SetCurrent(ctx, db, f.Project, v, f.BuildNumber); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "current release is now %s\n", f.Version)
	}
	return nil
}
