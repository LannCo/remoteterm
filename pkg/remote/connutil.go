// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/LannCo/remoteterm/pkg/blocklogger"
	"github.com/LannCo/remoteterm/pkg/genconn"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/util/iterfn"
	"github.com/LannCo/remoteterm/pkg/util/shellutil"
	"golang.org/x/crypto/ssh"
)

// wshCopyStallTimeout/wshCopyStallCheckInterval govern CpWshToRemote's
// binary-copy step: the transfer is killed only after this many seconds
// with no forward progress, not after a fixed wall-clock budget regardless
// of whether bytes are still moving (a slow-but-steady link should finish).
const (
	wshCopyStallTimeout       = 10 * time.Second
	wshCopyStallCheckInterval = 1 * time.Second
)

var userHostRe = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9._@\\-]*@)?([a-zA-Z0-9][a-zA-Z0-9.-]*)(?::([0-9]+))?$`)

func ParseOpts(input string) (*SSHOpts, error) {
	m := userHostRe.FindStringSubmatch(input)
	if m == nil {
		return nil, fmt.Errorf("invalid format of user@host argument")
	}
	remoteUser, remoteHost, remotePort := m[1], m[2], m[3]
	remoteUser = strings.Trim(remoteUser, "@")

	return &SSHOpts{SSHHost: remoteHost, SSHUser: remoteUser, SSHPort: remotePort}, nil
}

func normalizeOs(os string) string {
	os = strings.ToLower(strings.TrimSpace(os))
	return os
}

func normalizeArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "x86_64", "amd64":
		arch = "x64"
	case "arm64", "aarch64":
		arch = "arm64"
	}
	return arch
}

// returns (os, arch, error)
// guaranteed to return a supported platform
func GetClientPlatform(ctx context.Context, shell genconn.ShellClient) (string, string, error) {
	blocklogger.Infof(ctx, "[conndebug] running `uname -sm` to detect client platform\n")
	stdout, stderr, err := genconn.RunSimpleCommand(ctx, shell, genconn.CommandSpec{
		Cmd: "uname -sm",
	})
	if err != nil {
		return "", "", fmt.Errorf("error running uname -sm: %w, stderr: %s", err, stderr)
	}
	// Parse and normalize output
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(stdout)))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output from uname: %s", stdout)
	}
	os, arch := normalizeOs(parts[0]), normalizeArch(parts[1])
	if err := remotetermbase.ValidateWshSupportedArch(os, arch); err != nil {
		return "", "", err
	}
	return os, arch, nil
}

func GetClientPlatformFromOsArchStr(ctx context.Context, osArchStr string) (string, string, error) {
	parts := strings.Fields(strings.TrimSpace(osArchStr))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output from uname: %s", osArchStr)
	}
	os, arch := normalizeOs(parts[0]), normalizeArch(parts[1])
	if err := remotetermbase.ValidateWshSupportedArch(os, arch); err != nil {
		return "", "", err
	}
	return os, arch, nil
}

var installTemplateRawDefault = strings.TrimSpace(`
mkdir -p {{.installDir}} || exit 1;
cat > {{.tempPath}} || exit 1;
mv {{.tempPath}} {{.installPath}} || exit 1;
chmod a+x {{.installPath}} || exit 1;
`)
var installTemplate = template.Must(template.New("wsh-install-template").Parse(installTemplateRawDefault))

func CpWshToRemote(ctx context.Context, client *ssh.Client, clientOs string, clientArch string) error {
	deadline, ok := ctx.Deadline()
	if ok {
		blocklogger.Debugf(ctx, "[conndebug] CpWshToRemote, timeout: %v\n", time.Until(deadline))
	}
	wshLocalPath, err := shellutil.GetLocalWshBinaryPath(remotetermbase.WaveVersion, clientOs, clientArch)
	if err != nil {
		return err
	}
	input, err := os.Open(wshLocalPath)
	if err != nil {
		return fmt.Errorf("cannot open local file %s: %w", wshLocalPath, err)
	}
	defer input.Close()
	installWords := map[string]string{
		"installDir":  filepath.ToSlash(filepath.Dir(remotetermbase.RemoteFullWshBinPath)),
		"tempPath":    remotetermbase.RemoteFullWshBinPath + ".temp",
		"installPath": remotetermbase.RemoteFullWshBinPath,
	}
	var installCmd bytes.Buffer
	if err := installTemplate.Execute(&installCmd, installWords); err != nil {
		return fmt.Errorf("failed to prepare install command: %w", err)
	}
	blocklogger.Infof(ctx, "[conndebug] copying %q to remote server %q\n", wshLocalPath, remotetermbase.RemoteFullWshBinPath)
	genCmd, err := genconn.MakeSSHCmdClient(client, genconn.CommandSpec{
		Cmd: installCmd.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to create remote command: %w", err)
	}
	stdin, err := genCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	defer stdin.Close()
	stderrBuf, err := genconn.MakeStderrSyncBuffer(genCmd)
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	if err := genCmd.Start(); err != nil {
		return fmt.Errorf("failed to start remote command: %w", err)
	}

	// The copy is bounded by a stall timeout, not ctx's wall clock: kill it
	// only once wshCopyStallTimeout passes with no bytes written, so a
	// slow-but-moving link finishes instead of racing a fixed deadline.
	// stallCtx still ends if ctx itself is genuinely canceled (e.g. app
	// shutdown), just not by ctx's deadline.
	stallCtx, stallCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer stallCancel()
	tracker := genconn.MakeProgressTracker(nil)
	stallTicker := time.NewTicker(wshCopyStallCheckInterval)
	defer stallTicker.Stop()
	watchStop := make(chan struct{})
	defer close(watchStop)
	go genconn.WatchForStall(ctx, tracker, wshCopyStallTimeout, stallTicker.C, watchStop, stallCancel)

	copyDone := make(chan error, 1)
	go func() {
		defer close(copyDone)
		defer stdin.Close()
		progressStdin := &genconn.ProgressWriter{Dst: stdin, Tracker: tracker}
		if _, err := io.Copy(progressStdin, input); err != nil && err != io.EOF {
			copyDone <- fmt.Errorf("failed to copy data: %w", err)
		} else {
			copyDone <- nil
		}
	}()
	procErr := genconn.ProcessContextWait(stallCtx, genCmd)
	if procErr != nil {
		if stallCtx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("remote command failed: wsh binary copy stalled (no progress for %v): %w (stderr: %s)", wshCopyStallTimeout, procErr, stderrBuf.String())
		}
		return fmt.Errorf("remote command failed: %w (stderr: %s)", procErr, stderrBuf.String())
	}
	copyErr := <-copyDone
	if copyErr != nil {
		return fmt.Errorf("failed to copy data: %w (stderr: %s)", copyErr, stderrBuf.String())
	}
	return nil
}

func IsPowershell(shellPath string) bool {
	// get the base path, and then check contains
	shellBase := filepath.Base(shellPath)
	return strings.Contains(shellBase, "powershell") || strings.Contains(shellBase, "pwsh")
}

func NormalizeConfigPattern(pattern string) string {
	userName, err := WaveSshConfigUserSettings().GetStrict(pattern, "User")
	if err != nil || userName == "" {
		log.Printf("warning: error parsing username of %s for conn dropdown: %v", pattern, err)
		localUser, err := user.Current()
		if err == nil {
			userName = localUser.Username
		}
	}
	port, err := WaveSshConfigUserSettings().GetStrict(pattern, "Port")
	if err != nil {
		port = "22"
	}
	if userName != "" {
		userName += "@"
	}
	if port == "22" {
		port = ""
	} else {
		port = ":" + port
	}
	return fmt.Sprintf("%s%s%s", userName, pattern, port)
}

func ParseProfiles() []string {
	connfile, cerrs := rtconfig.ReadWaveHomeConfigFile(rtconfig.ProfilesFile)
	if len(cerrs) > 0 {
		log.Printf("error reading config file: %v", cerrs[0])
		return nil
	}

	return iterfn.MapKeysToSorted(connfile)
}
