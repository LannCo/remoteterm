// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermbase

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/util/utilfn"
)

// set by main-server.go
var WaveVersion = "0.0.0"
var BuildTime = "0"

const (
	WaveConfigHomeEnvVar           = "REMOTETERM_CONFIG_HOME"
	WaveDataHomeEnvVar             = "REMOTETERM_DATA_HOME"
	WaveAppPathVarName             = "REMOTETERM_APP_PATH"
	WaveAppResourcesPathVarName    = "REMOTETERM_RESOURCES_PATH"
	WaveAppElectronExecPathVarName = "REMOTETERM_ELECTRONEXECPATH"
	WaveDevVarName                 = "REMOTETERM_DEV"
	WaveDevViteVarName             = "REMOTETERM_DEV_VITE"
	WaveWshForceUpdateVarName      = "REMOTETERM_WSHFORCEUPDATE"
	WaveNoConfirmQuitVarName       = "REMOTETERM_NOCONFIRMQUIT"

	// The swap token itself is packed/unpacked entirely within a single wsh<->wavesrv round
	// trip of the same app version (the client reads whatever key the server packed, via a
	// generic map, not a hardcoded lookup) and is unset immediately in the same shell-startup
	// script that consumes it, so unlike the session-scoped vars below, it doesn't need a
	// legacy-name fallback.
	WaveSwapTokenVarName = "REMOTETERM_SWAPTOKEN"
)

// Session-scoped vars the app writes into every spawned shell's environment (directly, or via
// a swap-token exchange). Unlike the override vars above, a read-side fallback alone doesn't
// protect these: a shell/tmux pane spawned by an already-running pre-rename app instance has
// the old name baked into its inherited environment for the rest of its life. Write both names
// for one deprecation-window release with SetDualEnv, and read new-first via GetEnvNewOrLegacy.
const (
	// JWT is also exported directly into a spawned shell's persistent environment (both the
	// local swap-token exchange map and the direct env injection used for WSL/remote SSH
	// shell starts in pkg/shellexec/shellexec.go), and read back later via a hardcoded
	// os.Getenv/map lookup (cmd/wsh/cmd/wshcmd-root.go, pkg/remotetermapp/waveapp.go,
	// cmd/wsh/cmd/wshcmd-connserver.go) — unlike the swap token itself, it belongs in this
	// dual-write group, not the no-fallback-needed block above.
	WaveJwtTokenVarName       = "REMOTETERM_JWT"
	LegacyWaveJwtTokenVarName = "WAVETERM_JWT"

	WaveTabIdVarName       = "REMOTETERM_TABID"
	LegacyWaveTabIdVarName = "WAVETERM_TABID"

	WaveBlockIdVarName       = "REMOTETERM_BLOCKID"
	LegacyWaveBlockIdVarName = "WAVETERM_BLOCKID"

	WaveWorkspaceIdVarName       = "REMOTETERM_WORKSPACEID"
	LegacyWaveWorkspaceIdVarName = "WAVETERM_WORKSPACEID"

	WaveClientIdVarName       = "REMOTETERM_CLIENTID"
	LegacyWaveClientIdVarName = "WAVETERM_CLIENTID"

	WaveConnVarName       = "REMOTETERM_CONN"
	LegacyWaveConnVarName = "WAVETERM_CONN"

	WaveJobIdVarName       = "REMOTETERM_JOBID"
	LegacyWaveJobIdVarName = "WAVETERM_JOBID"

	WavePublicKeyVarName       = "REMOTETERM_PUBLICKEY"
	LegacyWavePublicKeyVarName = "WAVETERM_PUBLICKEY"

	// bare flag var (no trailing underscore) signaling "this shell is RemoteTerm-managed"; also
	// carries the wsh executable path in WaveshellLocalEnvVars
	WaveFlagVarName       = "REMOTETERM"
	LegacyWaveFlagVarName = "WAVETERM"

	WaveVersionVarName = "REMOTETERM_VERSION"
)

// SetDualEnv writes both the new and legacy names of a session-scoped env var for the
// deprecation window (see the const block above).
func SetDualEnv(env map[string]string, newName string, legacyName string, val string) {
	env[newName] = val
	env[legacyName] = val
}

// GetEnvNewOrLegacy reads a session-scoped env var, preferring the new name but falling back
// to the legacy name so wsh keeps working inside a shell pane spawned by a pre-rename app
// instance (see the const block above).
func GetEnvNewOrLegacy(newName string, legacyName string) string {
	if val := os.Getenv(newName); val != "" {
		return val
	}
	return os.Getenv(legacyName)
}

// GetMapValNewOrLegacy is GetEnvNewOrLegacy for an in-memory env map (e.g. a swap token's Env
// map that may have been created and persisted by a pre-rename app instance and only ever
// carries the legacy key).
func GetMapValNewOrLegacy(env map[string]string, newName string, legacyName string) string {
	if val := env[newName]; val != "" {
		return val
	}
	return env[legacyName]
}

const (
	BlockFile_Term  = "term"            // used for main pty output
	BlockFile_Cache = "cache:term:full" // for cached block
	BlockFile_VDom  = "vdom"            // used for alt html layout
	BlockFile_Env   = "env"
)

const NeedJwtConst = "NEED-JWT"

var ConfigHome_VarCache string          // caches REMOTETERM_CONFIG_HOME
var DataHome_VarCache string            // caches REMOTETERM_DATA_HOME
var AppPath_VarCache string             // caches REMOTETERM_APP_PATH
var AppResourcesPath_VarCache string    // caches REMOTETERM_RESOURCES_PATH
var AppElectronExecPath_VarCache string // caches REMOTETERM_ELECTRONEXECPATH
var Dev_VarCache string                 // caches REMOTETERM_DEV

const WaveLockFile = "remoteterm.lock"

// Frozen pre-rename lock name. emain's migration can rename a data dir out from under a running
// pre-rename server, which keeps holding this lock in the moved dir, so the server also takes it
// whenever it is present.
const LegacyWaveLockFile = "wave.lock"
const DomainSocketBaseName = "remoteterm.sock"
const RemoteDomainSocketBaseName = "wave-remote.sock"
const WaveDBDir = "db"
const ConfigDir = "config"
const RemoteWaveHomeDirName = ".waveterm"
const RemoteWshBinDirName = "bin"
const RemoteFullWshBinPath = "~/.waveterm/bin/wsh"
const RemoteFullDomainSocketPath = "~/.waveterm/wave-remote.sock"

const AppPathBinDir = "bin"

var baseLock = &sync.Mutex{}
var ensureDirCache = map[string]bool{}

var waveCachesDirOnce = &sync.Once{}
var waveCachesDir string

var SupportedWshBinaries = map[string]bool{
	"darwin-x64":    true,
	"darwin-arm64":  true,
	"linux-x64":     true,
	"linux-arm64":   true,
	"windows-x64":   true,
	"windows-arm64": true,
}

type FDLock interface {
	Close() error
}

type multiFDLock []FDLock

func (locks multiFDLock) Close() error {
	var errs []error
	for i := len(locks) - 1; i >= 0; i-- {
		if err := locks[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func AcquireWaveLock() (FDLock, error) {
	dataHomeDir := GetWaveDataDir()
	lock, err := acquireLockFile(filepath.Join(dataHomeDir, WaveLockFile))
	if err != nil {
		return nil, err
	}
	legacyLockFileName := filepath.Join(dataHomeDir, LegacyWaveLockFile)
	_, err = os.Stat(legacyLockFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return lock, nil
	}
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("cannot stat legacy lock %s: %w", legacyLockFileName, err)
	}
	legacyLock, err := acquireLockFile(legacyLockFileName)
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("legacy lock %s is held (a pre-rename instance is likely still running on this data dir): %w", legacyLockFileName, err)
	}
	return multiFDLock{lock, legacyLock}, nil
}

func CacheAndRemoveEnvVars() error {
	ConfigHome_VarCache = os.Getenv(WaveConfigHomeEnvVar)
	if ConfigHome_VarCache == "" {
		return fmt.Errorf(WaveConfigHomeEnvVar + " not set")
	}
	os.Unsetenv(WaveConfigHomeEnvVar)
	DataHome_VarCache = os.Getenv(WaveDataHomeEnvVar)
	if DataHome_VarCache == "" {
		return fmt.Errorf("%s not set", WaveDataHomeEnvVar)
	}
	os.Unsetenv(WaveDataHomeEnvVar)
	AppPath_VarCache = os.Getenv(WaveAppPathVarName)
	os.Unsetenv(WaveAppPathVarName)
	AppResourcesPath_VarCache = os.Getenv(WaveAppResourcesPathVarName)
	os.Unsetenv(WaveAppResourcesPathVarName)
	AppElectronExecPath_VarCache = os.Getenv(WaveAppElectronExecPathVarName)
	os.Unsetenv(WaveAppElectronExecPathVarName)
	Dev_VarCache = os.Getenv(WaveDevVarName)
	os.Unsetenv(WaveDevVarName)
	os.Unsetenv(WaveDevViteVarName)
	os.Unsetenv(WaveNoConfirmQuitVarName)
	return nil
}

func IsDevMode() bool {
	return Dev_VarCache != ""
}

func GetWaveAppPath() string {
	return AppPath_VarCache
}

func GetWaveAppResourcesPath() string {
	return AppResourcesPath_VarCache
}

func GetWaveDataDir() string {
	return DataHome_VarCache
}

func GetWaveConfigDir() string {
	return ConfigHome_VarCache
}

func GetWaveAppBinPath() string {
	return filepath.Join(GetWaveAppPath(), AppPathBinDir)
}

func GetWaveAppElectronExecPath() string {
	return AppElectronExecPath_VarCache
}

func GetHomeDir() string {
	homeVar, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return homeVar
}

func ExpandHomeDir(pathStr string) (string, error) {
	if pathStr != "~" && !strings.HasPrefix(pathStr, "~/") && (!strings.HasPrefix(pathStr, `~\`) || runtime.GOOS != "windows") {
		return filepath.Clean(pathStr), nil
	}
	homeDir := GetHomeDir()
	if pathStr == "~" {
		return homeDir, nil
	}
	expandedPath := filepath.Clean(filepath.Join(homeDir, pathStr[2:]))
	absPath, err := filepath.Abs(filepath.Join(homeDir, expandedPath))
	if err != nil || !strings.HasPrefix(absPath, homeDir) {
		return "", fmt.Errorf("potential path traversal detected for path %s", pathStr)
	}
	return expandedPath, nil
}

func ExpandHomeDirSafe(pathStr string) string {
	path, _ := ExpandHomeDir(pathStr)
	return path
}

func ReplaceHomeDir(pathStr string) string {
	homeDir := GetHomeDir()
	if pathStr == homeDir {
		return "~"
	}
	if strings.HasPrefix(pathStr, homeDir+"/") {
		return "~" + pathStr[len(homeDir):]
	}
	return pathStr
}

func GetDomainSocketName() string {
	return filepath.Join(GetWaveDataDir(), DomainSocketBaseName)
}

// returns a Unix-style path for the remote socket (using fmt.Sprintf instead of filepath.Join
// because this path is for a remote Unix system, not the local OS which might be Windows)
func GetPersistentRemoteSockName(clientId string) string {
	return fmt.Sprintf("~/.waveterm/client/%s/waveterm.sock", clientId)
}

func EnsureWaveDataDir() error {
	return CacheEnsureDir(GetWaveDataDir(), "wavehome", 0700, "wave home directory")
}

func EnsureWaveDBDir() error {
	return CacheEnsureDir(filepath.Join(GetWaveDataDir(), WaveDBDir), "wavedb", 0700, "wave db directory")
}

func EnsureWaveConfigDir() error {
	return CacheEnsureDir(GetWaveConfigDir(), "waveconfig", 0700, "wave config directory")
}

func EnsureWavePresetsDir() error {
	return CacheEnsureDir(filepath.Join(GetWaveConfigDir(), "presets"), "wavepresets", 0700, "wave presets directory")
}

func resolveWaveCachesDir() string {
	var cacheDir string
	appBundle := "waveterm"
	if IsDevMode() {
		appBundle = "waveterm-dev"
	}

	switch runtime.GOOS {
	case "darwin":
		homeDir := GetHomeDir()
		cacheDir = filepath.Join(homeDir, "Library", "Caches", appBundle)
	case "linux":
		xdgCache := os.Getenv("XDG_CACHE_HOME")
		if xdgCache != "" {
			cacheDir = filepath.Join(xdgCache, appBundle)
		} else {
			homeDir := GetHomeDir()
			cacheDir = filepath.Join(homeDir, ".cache", appBundle)
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			cacheDir = filepath.Join(localAppData, appBundle, "Cache")
		}
	}

	if cacheDir == "" {
		tmpDir := os.TempDir()
		cacheDir = filepath.Join(tmpDir, appBundle)
	}

	return cacheDir
}

func GetWaveCachesDir() string {
	waveCachesDirOnce.Do(func() {
		waveCachesDir = resolveWaveCachesDir()
	})
	return waveCachesDir
}

func EnsureWaveCachesDir() error {
	return CacheEnsureDir(GetWaveCachesDir(), "wavecaches", 0700, "wave caches directory")
}

func CacheEnsureDir(dirName string, cacheKey string, perm os.FileMode, dirDesc string) error {
	baseLock.Lock()
	ok := ensureDirCache[cacheKey]
	baseLock.Unlock()
	if ok {
		return nil
	}
	err := TryMkdirs(dirName, perm, dirDesc)
	if err != nil {
		return err
	}
	baseLock.Lock()
	ensureDirCache[cacheKey] = true
	baseLock.Unlock()
	return nil
}

func TryMkdirs(dirName string, perm os.FileMode, dirDesc string) error {
	info, err := os.Stat(dirName)
	if errors.Is(err, fs.ErrNotExist) {
		err = os.MkdirAll(dirName, perm)
		if err != nil {
			return fmt.Errorf("cannot make %s %q: %w", dirDesc, dirName, err)
		}
		info, err = os.Stat(dirName)
	}
	if err != nil {
		return fmt.Errorf("error trying to stat %s: %w", dirDesc, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s %q must be a directory", dirDesc, dirName)
	}
	return nil
}

func listValidLangs(ctx context.Context) []string {
	out, err := exec.CommandContext(ctx, "locale", "-a").CombinedOutput()
	if err != nil {
		log.Printf("error running 'locale -a': %s\n", err)
		return []string{}
	}
	// don't bother with CRLF line endings
	// this command doesn't work on windows
	return strings.Split(string(out), "\n")
}

var osLangOnce = &sync.Once{}
var osLang string

func determineLang() string {
	defaultLang := "en_US.UTF-8"
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	if runtime.GOOS == "darwin" {
		out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLocale").CombinedOutput()
		if err != nil {
			log.Printf("error executing 'defaults read -g AppleLocale', will use default 'en_US.UTF-8': %v\n", err)
			return defaultLang
		}
		strOut := string(out)
		truncOut := strings.Split(strOut, "@")[0]
		preferredLang := strings.TrimSpace(truncOut) + ".UTF-8"
		validLangs := listValidLangs(ctx)

		if !utilfn.ContainsStr(validLangs, preferredLang) {
			log.Printf("unable to use desired lang %s, will use default 'en_US.UTF-8'\n", preferredLang)
			return defaultLang
		}

		return preferredLang
	} else {
		// this is specifically to get the wavesrv LANG so waveshell
		// on a remote uses the same LANG
		return os.Getenv("LANG")
	}
}

func DetermineLang() string {
	osLangOnce.Do(func() {
		osLang = determineLang()
	})
	return osLang
}

func DetermineLocale() string {
	truncated := strings.Split(DetermineLang(), ".")[0]
	if truncated == "" {
		return "C"
	}
	return strings.Replace(truncated, "_", "-", -1)
}

func ClientArch() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func ClientPackageType() string {
	if os.Getenv("SNAP") != "" {
		return "snap"
	}
	if os.Getenv("APPIMAGE") != "" {
		return "appimage"
	}
	return ""
}

var macOSVersionOnce = &sync.Once{}
var cachedMacOSVersion string

var macOSVersionRegex = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)

func internalMacOSVersion() string {
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return ""
	}
	versionStr := strings.TrimSpace(string(out))
	m := macOSVersionRegex.FindStringSubmatch(versionStr)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func ClientMacOSVersion() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	macOSVersionOnce.Do(func() {
		cachedMacOSVersion = internalMacOSVersion()
	})
	return cachedMacOSVersion
}

var releaseRegex = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
var osReleaseOnce = &sync.Once{}
var osRelease string

func unameKernelRelease() string {
	if runtime.GOOS == "windows" {
		return "-"
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	out, err := exec.CommandContext(ctx, "uname", "-r").CombinedOutput()
	if err != nil {
		log.Printf("error executing uname -r: %v\n", err)
		return "-"
	}
	releaseStr := strings.TrimSpace(string(out))
	m := releaseRegex.FindStringSubmatch(releaseStr)
	if len(m) < 2 {
		log.Printf("invalid uname -r output: [%s]\n", releaseStr)
		return "-"
	}
	return m[1]
}

func UnameKernelRelease() string {
	osReleaseOnce.Do(func() {
		osRelease = unameKernelRelease()
	})
	return osRelease
}

var systemSummaryOnce = &sync.Once{}
var systemSummary string

func GetSystemSummary() string {
	systemSummaryOnce.Do(func() {
		ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelFn()
		systemSummary = getSystemSummary(ctx)
	})
	return systemSummary
}

func ValidateWshSupportedArch(os string, arch string) error {
	if SupportedWshBinaries[fmt.Sprintf("%s-%s", os, arch)] {
		return nil
	}
	return fmt.Errorf("unsupported wsh platform: %s-%s", os, arch)
}

func getSystemSummary(ctx context.Context) string {
	osName := runtime.GOOS

	switch osName {
	case "darwin":
		out, _ := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
		return fmt.Sprintf("macOS %s (%s)", strings.TrimSpace(string(out)), runtime.GOARCH)
	case "linux":
		// Read /etc/os-release directly (standard location since 2012)
		data, err := os.ReadFile("/etc/os-release")
		var prettyName string
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					prettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					break
				}
			}
		}
		if prettyName == "" {
			prettyName = "Linux"
		} else if !strings.Contains(strings.ToLower(prettyName), "linux") {
			prettyName = "Linux " + prettyName
		}
		return fmt.Sprintf("%s (%s)", prettyName, runtime.GOARCH)
	case "windows":
		var details string
		out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_OperatingSystem).Caption").Output()
		if err == nil && len(out) > 0 {
			details = strings.TrimSpace(string(out))
		} else {
			details = "Windows"
		}
		return fmt.Sprintf("%s (%s)", details, runtime.GOARCH)
	default:
		return fmt.Sprintf("%s (%s)", runtime.GOOS, runtime.GOARCH)
	}
}

// job socket path on remote machine
func GetRemoteJobSocketPath(jobId string) string {
	socketDir := filepath.Join("/tmp", fmt.Sprintf("waveterm-%d", os.Getuid()))
	return filepath.Join(socketDir, fmt.Sprintf("%s.sock", jobId))
}

// job file path on remote machine
func GetRemoteJobFilePath(jobId string, extension string) string {
	jobDir := GetRemoteJobLogDir()
	return filepath.Join(jobDir, fmt.Sprintf("%s.%s", jobId, extension))
}

// job file dir on remote machines
func GetRemoteJobLogDir() string {
	homeDir := GetHomeDir()
	jobDir := filepath.Join(homeDir, ".waveterm", "jobs")
	return jobDir
}
