// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package rtstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
)

const (
	MetaKey_BuilderOwner = "builder:owner"
	MetaKey_BuilderAppId = "builder:appid"
	BuilderMetaPrefix    = "builder:"
)

const maxBlockParentDepth = 5

var ErrBuilderLocalOnly = errors.New("builder terminals are local only: remote connections are not available in a builder window")

// IsBuilderTab reports whether tabId is a builder-owned tab: it carries builder:owner and belongs to
// no workspace. A lookup error counts as "not a builder tab", so callers that delete refuse.
// Inside a transaction pass tx.Context(): the DB has one connection, so an outer context would block.
func IsBuilderTab(ctx context.Context, tabId string) bool {
	tab, err := DBGet[*remotetermobj.Tab](ctx, tabId)
	if err != nil || tab == nil {
		return false
	}
	if tab.Meta.GetString(MetaKey_BuilderOwner, "") == "" {
		return false
	}
	wsId, err := DBFindWorkspaceForTabId(ctx, tabId)
	if err != nil {
		return false
	}
	return wsId == ""
}

// builderId "" matches any non-empty owner. Results still need IsBuilderTab: a workspace tab can carry the key.
func DBFindTabIdsByBuilderOwner(ctx context.Context, builderId string) ([]string, error) {
	return WithTxRtn(ctx, func(tx *TxWrap) ([]string, error) {
		if builderId == "" {
			query := `SELECT oid FROM db_tab WHERE COALESCE(json_extract(data, '$.meta."builder:owner"'), '') <> ''`
			return tx.SelectStrings(query), nil
		}
		query := `SELECT oid FROM db_tab WHERE json_extract(data, '$.meta."builder:owner"') = ?`
		return tx.SelectStrings(query, builderId), nil
	})
}

func CheckNoReservedTabMeta(meta remotetermobj.MetaMapType) error {
	for key := range meta {
		if strings.HasPrefix(key, BuilderMetaPrefix) {
			return fmt.Errorf("tab meta key %q is reserved", key)
		}
	}
	return nil
}

func isLocalConnMetaValue(val any) bool {
	if val == nil {
		return true
	}
	connName, ok := val.(string)
	if !ok {
		return false
	}
	return connName == "" || connName == "local" || strings.HasPrefix(connName, "local:")
}

// The connection checks look anything up only when the patch sets a connection. They keep SSH
// prompts out of builder windows, which cannot show them; they are not a security boundary.
func CheckBuilderTabConnection(ctx context.Context, tabId string, meta remotetermobj.MetaMapType) error {
	connVal, ok := meta[remotetermobj.MetaKey_Connection]
	if !ok || isLocalConnMetaValue(connVal) {
		return nil
	}
	if !IsBuilderTab(ctx, tabId) {
		return nil
	}
	return ErrBuilderLocalOnly
}

func CheckBuilderBlockConnection(ctx context.Context, blockId string, meta remotetermobj.MetaMapType) error {
	connVal, ok := meta[remotetermobj.MetaKey_Connection]
	if !ok || isLocalConnMetaValue(connVal) {
		return nil
	}
	tabId := dbFindTabForBlockIdQuiet(ctx, blockId)
	if tabId == "" {
		return nil
	}
	return CheckBuilderTabConnection(ctx, tabId, meta)
}

// Unlike DBFindTabForBlockId this never returns an error. Inside a transaction a nested error marks
// the shared TxWrap failed, which would roll back the caller's write for an unrelated reason.
func dbFindTabForBlockIdQuiet(ctx context.Context, blockId string) string {
	tabId, _ := WithTxRtn(ctx, func(tx *TxWrap) (string, error) {
		for range maxBlockParentDepth {
			parentORef := tx.GetString(`SELECT json_extract(data, '$.parentoref') FROM db_block WHERE oid = ?`, blockId)
			oref, err := remotetermobj.ParseORef(parentORef)
			if err != nil {
				return "", nil
			}
			if oref.OType == remotetermobj.OType_Tab {
				return oref.OID, nil
			}
			if oref.OType != remotetermobj.OType_Block {
				return "", nil
			}
			blockId = oref.OID
		}
		return "", nil
	})
	return tabId
}
