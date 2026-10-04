// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"testing"

	"golang.org/x/mod/modfile"
)

func parseGoMod(t *testing.T, data []byte) *modfile.File {
	t.Helper()
	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("generated go.mod does not parse: %v\n%s", err, data)
	}
	return mf
}

func TestMakeGoModContentQuotesPathWithSpace(t *testing.T) {
	sdkPath := "/Program Files/RemoteTerm/resources/tsunamisdk"
	data, err := makeGoModContent(nil, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.25.6",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: sdkPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	mf := parseGoMod(t, data)
	if mf.Module.Mod.Path != "tsunami/draft/demo" || mf.Go.Version != "1.25.6" {
		t.Fatalf("module %q go %q", mf.Module.Mod.Path, mf.Go.Version)
	}
	if len(mf.Replace) != 1 || mf.Replace[0].New.Path != sdkPath {
		t.Fatalf("replace = %+v", mf.Replace)
	}
}

func TestMakeGoModContentRewritesStaleReplace(t *testing.T) {
	existing := []byte("module tsunami/draft/demo\n\ngo 1.22\n\nrequire github.com/LannCo/remoteterm/tsunami v0.12.4\n\nreplace github.com/LannCo/remoteterm/tsunami => /tmp/.mount_RemoteOLD/resources/tsunamisdk\n")
	data, err := makeGoModContent(existing, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.26.3",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: "/tmp/.mount_RemoteNEW/resources/tsunamisdk",
	})
	if err != nil {
		t.Fatal(err)
	}
	mf := parseGoMod(t, data)
	if len(mf.Replace) != 1 || mf.Replace[0].New.Path != "/tmp/.mount_RemoteNEW/resources/tsunamisdk" {
		t.Fatalf("replace = %+v", mf.Replace)
	}
	if mf.Go.Version != "1.25.6" {
		t.Fatalf("go line = %q, want it raised to the floor 1.25.6", mf.Go.Version)
	}
}

func TestMakeGoModContentKeepsNewerGoLine(t *testing.T) {
	existing := []byte("module tsunami/draft/demo\n\ngo 1.26\n")
	data, err := makeGoModContent(existing, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.26.3",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: "/sdk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mf := parseGoMod(t, data); mf.Go.Version != "1.26" {
		t.Fatalf("go line = %q, want 1.26 unchanged", mf.Go.Version)
	}
}
