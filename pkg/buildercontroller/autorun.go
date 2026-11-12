// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
)

// The frontend matches on this code, as it does on ERR-SECRET.
const AutoRunDeclinedCode = "ERR-AUTORUN-DECLINED"

var ErrAutoRunDeclined = errors.New(AutoRunDeclinedCode + ": the app has changed since you last started it")

// Opening an app must not compile and run whatever is on disk: building executes the result
// (the manifest run), with the app's bound secrets. So an automatic start is allowed only for
// inputs identical to those of the last build the user started by hand (Start, Rebuild, or a
// Code-tab save). Everything else waits for the Start button.
type buildTrust struct {
	appId string
	base  string
}

// go.mod and go.sum decide which module code is compiled in (a replace directive can point
// at any directory), yet the watcher's input hash leaves them out because the build rewrites
// them. The trust hash adds them, and a build that rewrote them is followed by a refresh.
func computeTrustHash(appDir string) (base string, full string, err error) {
	base, err = ComputeAppInputHash(appDir)
	if err != nil {
		return "", "", err
	}
	root, err := remotetermappstore.OpenAppRoot(appDir)
	if err != nil {
		return "", "", fmt.Errorf("cannot scan app folder %s: %w", appDir, err)
	}
	defer root.Close()
	hasher := sha256.New()
	hasher.Write([]byte(base))
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := remotetermappstore.ReadAppRootFile(root, name, remotetermappstore.MaxAppFileReadSize)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Unreadable counts as different content, never as absent.
			data = []byte("unreadable:" + err.Error())
		}
		fmt.Fprintf(hasher, "\x00%s\x00%d\x00", name, len(data))
		hasher.Write(data)
	}
	return base, hex.EncodeToString(hasher.Sum(nil)), nil
}

// A user-initiated rebuild: the same as RequestRebuild, and the inputs become the trusted ones.
func (bc *BuilderController) RequestUserRebuild(appId string, builderEnv map[string]string) {
	bc.requestRebuild(appId, builderEnv, true)
}

// The open-time start. It builds only when the current inputs are the trusted ones.
func (bc *BuilderController) RequestAutoRun(appId string, builderEnv map[string]string) error {
	if !bc.acceptsRequests() {
		return nil
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return err
	}
	base, full, err := computeTrustHash(appDir)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrAutoRunDeclined, err)
	}
	trusted := remotetermappstore.ReadTrustedBuildHash(appId)
	if trusted == "" || trusted != full {
		return ErrAutoRunDeclined
	}
	bc.setBuildTrust(buildTrust{appId: appId, base: base})
	bc.requestRebuild(appId, builderEnv, false)
	return nil
}

func (bc *BuilderController) trustCurrentInputs(appId string) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	base, full, err := computeTrustHash(appDir)
	if err != nil {
		log.Printf("BuilderController: cannot record trusted inputs for %s: %v\n", appId, err)
		return
	}
	if err := remotetermappstore.WriteTrustedBuildHash(appId, full); err != nil {
		log.Printf("BuilderController: cannot record trusted inputs for %s: %v\n", appId, err)
		return
	}
	bc.setBuildTrust(buildTrust{appId: appId, base: base})
}

// Only while the .go files and static/ are still what the user started: a build that
// rewrote go.mod keeps its trust, an edit that landed during the build does not.
func (bc *BuilderController) refreshTrustAfterBuild(appId string) {
	trust := bc.getBuildTrust()
	if trust.appId != appId || trust.base == "" {
		return
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	base, full, err := computeTrustHash(appDir)
	if err != nil || base != trust.base {
		return
	}
	if remotetermappstore.ReadTrustedBuildHash(appId) == full {
		return
	}
	if err := remotetermappstore.WriteTrustedBuildHash(appId, full); err != nil {
		log.Printf("BuilderController: cannot refresh trusted inputs for %s: %v\n", appId, err)
	}
}

func (bc *BuilderController) setBuildTrust(trust buildTrust) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.trust = trust
}

func (bc *BuilderController) getBuildTrust() buildTrust {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.trust
}
