// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package objectservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtcore"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/tsgen/tsgenmeta"
	"github.com/LannCo/remoteterm/pkg/wps"
)

type ObjectService struct{}

const DefaultTimeout = 2 * time.Second
const ConnContextTimeout = 60 * time.Second

func parseORef(oref string) (*remotetermobj.ORef, error) {
	fields := strings.Split(oref, ":")
	if len(fields) != 2 {
		return nil, fmt.Errorf("invalid object reference: %q", oref)
	}
	return &remotetermobj.ORef{OType: fields[0], OID: fields[1]}, nil
}

func (svc *ObjectService) GetObject_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "get wave object by oref",
		ArgNames: []string{"oref"},
	}
}

func (svc *ObjectService) GetObject(orefStr string) (remotetermobj.WaveObj, error) {
	oref, err := parseORef(orefStr)
	if err != nil {
		return nil, err
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	obj, err := rtstore.DBGetORef(ctx, *oref)
	if err != nil {
		return nil, fmt.Errorf("error getting object: %w", err)
	}
	return obj, nil
}

func (svc *ObjectService) GetObjects_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames:   []string{"orefs"},
		ReturnDesc: "objects",
	}
}

func (svc *ObjectService) GetObjects(orefStrArr []string) ([]remotetermobj.WaveObj, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()

	var orefArr []remotetermobj.ORef
	for _, orefStr := range orefStrArr {
		orefObj, err := parseORef(orefStr)
		if err != nil {
			return nil, err
		}
		orefArr = append(orefArr, *orefObj)
	}
	return rtstore.DBSelectORefs(ctx, orefArr)
}

func (svc *ObjectService) CreateBlock_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames:   []string{"uiContext", "blockDef", "rtOpts"},
		ReturnDesc: "blockId",
	}
}

func (svc *ObjectService) CreateBlock(uiContext remotetermobj.UIContext, blockDef *remotetermobj.BlockDef, rtOpts *remotetermobj.RuntimeOpts) (string, remotetermobj.UpdatesRtnType, error) {
	if uiContext.ActiveTabId == "" {
		return "", nil, fmt.Errorf("no active tab")
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	ctx = remotetermobj.ContextWithUpdates(ctx)

	blockData, err := rtcore.CreateBlock(ctx, uiContext.ActiveTabId, blockDef, rtOpts)
	if err != nil {
		return "", nil, err
	}

	return blockData.OID, remotetermobj.ContextGetUpdatesRtn(ctx), nil
}

func (svc *ObjectService) DeleteBlock_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames: []string{"uiContext", "blockId"},
	}
}

func (svc *ObjectService) DeleteBlock(uiContext remotetermobj.UIContext, blockId string) (remotetermobj.UpdatesRtnType, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	ctx = remotetermobj.ContextWithUpdates(ctx)
	err := rtcore.DeleteBlock(ctx, blockId, true)
	if err != nil {
		return nil, fmt.Errorf("error deleting block: %w", err)
	}
	return remotetermobj.ContextGetUpdatesRtn(ctx), nil
}

func (svc *ObjectService) UpdateObjectMeta_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames: []string{"uiContext", "oref", "meta"},
	}
}

func (svc *ObjectService) UpdateObjectMeta(uiContext remotetermobj.UIContext, orefStr string, meta remotetermobj.MetaMapType) (remotetermobj.UpdatesRtnType, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	ctx = remotetermobj.ContextWithUpdates(ctx)
	oref, err := parseORef(orefStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing object reference: %w", err)
	}
	err = rtstore.UpdateObjectMeta(ctx, *oref, meta, false)
	if err != nil {
		return nil, fmt.Errorf("error updating %q meta: %w", orefStr, err)
	}
	return remotetermobj.ContextGetUpdatesRtn(ctx), nil
}

func (svc *ObjectService) UpdateObject_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames: []string{"uiContext", "waveObj", "returnUpdates"},
	}
}

func (svc *ObjectService) UpdateObject(uiContext remotetermobj.UIContext, waveObj remotetermobj.WaveObj, returnUpdates bool) (remotetermobj.UpdatesRtnType, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	ctx = remotetermobj.ContextWithUpdates(ctx)
	if waveObj == nil {
		return nil, fmt.Errorf("update wavobj is nil")
	}
	oref := remotetermobj.ORefFromWaveObj(waveObj)
	found, err := rtstore.DBExistsORef(ctx, *oref)
	if err != nil {
		return nil, fmt.Errorf("error getting object: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("object not found: %s", oref)
	}
	err = rtstore.DBUpdate(ctx, waveObj)
	if err != nil {
		return nil, fmt.Errorf("error updating object: %w", err)
	}
	if (waveObj.GetOType() == remotetermobj.OType_Workspace) && (waveObj.(*remotetermobj.Workspace).Name != "") {
		wps.Broker.Publish(wps.WaveEvent{
			Event: wps.Event_WorkspaceUpdate})
	}
	if returnUpdates {
		return remotetermobj.ContextGetUpdatesRtn(ctx), nil
	}
	return nil, nil
}
