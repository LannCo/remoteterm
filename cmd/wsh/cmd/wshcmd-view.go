// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/spf13/cobra"
)

var viewMagnified bool

var viewCmd = &cobra.Command{
	Use:     "view {file|directory|URL}",
	Aliases: []string{"preview", "open"},
	Short:   "preview/edit a file or directory",
	RunE:    viewRun,
	PreRunE: preRunSetupRpcClient,
}

var editCmd = &cobra.Command{
	Use:     "edit {file}",
	Short:   "edit a file",
	RunE:    viewRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	viewCmd.Flags().BoolVarP(&viewMagnified, "magnified", "m", false, "open view in magnified mode")
	rootCmd.AddCommand(viewCmd)
	editCmd.Flags().BoolVarP(&viewMagnified, "magnified", "m", false, "open view in magnified mode")
	rootCmd.AddCommand(editCmd)
}

func viewRun(cmd *cobra.Command, args []string) (rtnErr error) {
	cmdName := cmd.Name()
	defer func() {
	}()
	if len(args) == 0 {
		OutputHelpMessage(cmd)
		return fmt.Errorf("no arguments.  wsh %s requires a file or URL as an argument argument", cmdName)
	}
	if len(args) > 1 {
		OutputHelpMessage(cmd)
		return fmt.Errorf("too many arguments.  wsh %s requires exactly one argument", cmdName)
	}
	tabId := getTabIdFromEnv()
	if tabId == "" {
		return fmt.Errorf("no REMOTETERM_TABID env var set")
	}
	fileArg := args[0]
	conn := RpcContext.Conn
	var wshCmd *wshrpc.CommandCreateBlockData
	if strings.HasPrefix(fileArg, "http://") || strings.HasPrefix(fileArg, "https://") {
		wshCmd = &wshrpc.CommandCreateBlockData{
			TabId: tabId,
			BlockDef: &remotetermobj.BlockDef{
				Meta: map[string]any{
					remotetermobj.MetaKey_View: "web",
					remotetermobj.MetaKey_Url:  fileArg,
				},
			},
			Magnified: viewMagnified,
			Focused:   true,
		}
	} else {
		absFile, err := filepath.Abs(fileArg)
		if err != nil {
			return fmt.Errorf("getting absolute path: %w", err)
		}
		absParent, err := filepath.Abs(filepath.Dir(fileArg))
		if err != nil {
			return fmt.Errorf("getting absolute path of parent dir: %w", err)
		}
		_, err = os.Stat(absParent)
		if err == fs.ErrNotExist {
			return fmt.Errorf("parent directory does not exist: %q", absParent)
		}
		if err != nil {
			return fmt.Errorf("getting file info: %w", err)
		}
		wshCmd = &wshrpc.CommandCreateBlockData{
			TabId: tabId,
			BlockDef: &remotetermobj.BlockDef{
				Meta: map[string]interface{}{
					remotetermobj.MetaKey_View: "preview",
					remotetermobj.MetaKey_File: absFile,
				},
			},
			Magnified: viewMagnified,
			Focused:   true,
		}
		if cmdName == "edit" {
			wshCmd.BlockDef.Meta[remotetermobj.MetaKey_Edit] = true
		}
		if conn != "" {
			wshCmd.BlockDef.Meta[remotetermobj.MetaKey_Connection] = conn
		}
	}
	_, err := wshclient.CreateBlockCommand(RpcClient, *wshCmd, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("running view command: %w", err)
	}
	return nil
}
