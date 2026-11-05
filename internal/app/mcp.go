package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	"github.com/babarot/gh-desk/internal/github"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/mcpserver"
	"github.com/babarot/gh-desk/internal/model"
)

// The environment Claude Code is run in for a conversation about a pull
// request (claude.prEnv), which the MCP servers it starts inherit.
const (
	envOwner  = "GH_DESK_OWNER"
	envRepo   = "GH_DESK_REPO"
	envNumber = "GH_DESK_NUMBER"
)

// serveMCP is gh desk mcp: gh-desk's own MCP server, which Claude Code
// starts for a conversation and talks to over stdin and stdout (ADR-078).
// The pull request is read from the environment gh-desk runs Claude Code
// in, never from what Claude sends. stdout belongs to the protocol;
// nothing else is written there.
func serveMCP(o *options, stderr io.Writer) int {
	level := new(slog.LevelVar)
	// The TUI that started Claude Code rotates the log; two processes
	// moving it aside at once would lose lines.
	log, closeLog, err := logging.Open(logging.Options{Level: level, NoRotate: true})
	if err != nil {
		fmt.Fprintf(stderr, "gh-desk mcp: open the log file: %v\n", err)
		return 1
	}
	defer closeLog.Close()
	log = log.With(logging.KeyRun, logging.NewID())
	if _, _, _, err := load(o.config, o.debug, level); err != nil {
		log.Error("mcp: config", "err", err.Error())
		fmt.Fprintln(stderr, err)
		return 1
	}
	key, err := prFromEnv(os.Getenv)
	if err != nil {
		log.Error("mcp: no pull request", "err", err.Error())
		fmt.Fprintf(stderr, "gh-desk mcp: %v\n", err)
		return 1
	}
	gh, err := github.New(log, github.Options{})
	if err != nil {
		log.Error("mcp: github client", "err", err.Error())
		fmt.Fprintf(stderr, "gh-desk mcp: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log.Debug("mcp start", logging.KeyPR, key.String())
	if err := mcpserver.New(gh, key, log).Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("mcp", logging.KeyPR, key.String(), "err", err.Error())
		return 1
	}
	return 0
}

// prFromEnv is the pull request a conversation is about, from the
// environment gh-desk runs Claude Code in.
func prFromEnv(getenv func(string) string) (model.PRKey, error) {
	owner, repo := getenv(envOwner), getenv(envRepo)
	n, err := strconv.Atoi(getenv(envNumber))
	if owner == "" || repo == "" || err != nil || n <= 0 {
		return model.PRKey{}, fmt.Errorf("%s, %s and %s name the pull request; gh-desk sets them when it starts Claude Code", envOwner, envRepo, envNumber)
	}
	return model.PRKey{Owner: owner, Repo: repo, Number: n}, nil
}
