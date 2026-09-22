// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jobcontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermjwt"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/LannCo/remoteterm/pkg/wshutil"
	"github.com/google/uuid"
)

var (
	storeFixtureOnce sync.Once
	storeFixtureDir  string
	storeFixtureErr  error
	statusRecorder   = &blockStatusRecorder{}
)

func TestMain(m *testing.M) {
	code := m.Run()
	if storeFixtureDir != "" {
		os.RemoveAll(storeFixtureDir)
	}
	os.Exit(code)
}

// blockStatusRecorder captures BlockJobStatus events published through wps.Broker.
type blockStatusRecorder struct {
	lock   sync.Mutex
	events []*wshrpc.BlockJobStatusData
}

func (r *blockStatusRecorder) SendEvent(routeId string, ev wps.WaveEvent) {
	if ev.Event != wps.Event_BlockJobStatus {
		return
	}
	data, ok := ev.Data.(*wshrpc.BlockJobStatusData)
	if !ok {
		return
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.events = append(r.events, data)
}

func (r *blockStatusRecorder) statusesForBlock(blockId string) []string {
	r.lock.Lock()
	defer r.lock.Unlock()
	var rtn []string
	for _, ev := range r.events {
		if ev.BlockId == blockId {
			rtn = append(rtn, ev.Status)
		}
	}
	return rtn
}

func lastStatusForBlock(blockId string) string {
	statuses := statusRecorder.statusesForBlock(blockId)
	if len(statuses) == 0 {
		return ""
	}
	return statuses[len(statuses)-1]
}

// initStoreFixture sets up a real temp wstore/filestore, JWT keys, a router with
// the bare RPC client, and a wps subscriber recording block job status events.
// Tests using it drive the real reconnect code paths against fake job/conn routes.
func initStoreFixture(t *testing.T) {
	t.Helper()
	storeFixtureOnce.Do(func() {
		dir, err := os.MkdirTemp("", "jobcontroller-test")
		if err != nil {
			storeFixtureErr = err
			return
		}
		storeFixtureDir = dir
		if err := os.MkdirAll(filepath.Join(dir, remotetermbase.WaveDBDir), 0755); err != nil {
			storeFixtureErr = err
			return
		}
		remotetermbase.DataHome_VarCache = dir
		if err := rtstore.InitWStore(); err != nil {
			storeFixtureErr = fmt.Errorf("wstore: %w", err)
			return
		}
		if err := filestore.InitFilestore(); err != nil {
			storeFixtureErr = fmt.Errorf("filestore: %w", err)
			return
		}
		kp, err := remotetermjwt.GenerateKeyPair()
		if err != nil {
			storeFixtureErr = err
			return
		}
		remotetermjwt.SetPrivateKey(kp.PrivateKey)
		remotetermjwt.SetPublicKey(kp.PublicKey)
		wshutil.DefaultRouter = wshutil.NewWshRouter()
		wshclient.GetBareRpcClient()
		wps.Broker.Subscribe("jobcontroller-test", wps.SubscriptionRequest{Event: wps.Event_BlockJobStatus, AllScopes: true})
		wps.Broker.SetClient(statusRecorder)
	})
	if storeFixtureErr != nil {
		t.Fatalf("store fixture: %v", storeFixtureErr)
	}
}

// makeRunningLocalJob inserts a running durable job on the "local" connection
// (IsConnected is always true for local) attached to a fresh block.
func makeRunningLocalJob(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	jobId := uuid.New().String()
	blockId := uuid.New().String()
	if err := rtstore.DBInsert(ctx, &remotetermobj.Block{OID: blockId, JobId: jobId, Meta: remotetermobj.MetaMapType{}}); err != nil {
		t.Fatalf("insert block: %v", err)
	}
	job := &remotetermobj.Job{
		OID:              jobId,
		Connection:       "local",
		JobKind:          JobKind_Shell,
		Cmd:              "bash",
		JobManagerStatus: JobManagerStatus_Running,
		AttachedBlockId:  blockId,
		CmdTermSize:      remotetermobj.TermSize{Rows: 24, Cols: 80},
		Meta:             remotetermobj.MetaMapType{},
	}
	if err := rtstore.DBInsert(ctx, job); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		SetJobConnStatus(jobId, JobConnStatus_Disconnected)
		if reader, ok := jobReaders.GetEx(jobId); ok && reader != nil {
			reader.Close()
		}
	})
	return jobId, blockId
}

// TestConnectedNoStreamRestartFailurePublishesStatus: when the Connected-but-no-stream
// restart fails, doReconnectJob marks the job Disconnected; the block must be told,
// otherwise the UI keeps showing the "connected" it last saw.
func TestConnectedNoStreamRestartFailurePublishesStatus(t *testing.T) {
	initStoreFixture(t)
	ctx := context.Background()
	jobId, blockId := makeRunningLocalJob(t)
	SetJobConnStatus(jobId, JobConnStatus_Connected)
	SendBlockJobStatusEvent(ctx, blockId)
	if got := lastStatusForBlock(blockId); got != "connected" {
		t.Fatalf("precondition: expected published status connected, got %q", got)
	}

	// No job route is registered, so restartStreaming's JobPrepareConnect fails.
	err := ReconnectJob(ctx, jobId, nil)
	if err == nil {
		t.Fatalf("expected ReconnectJob to fail with no job route")
	}
	if got := GetJobConnStatus(jobId); got != JobConnStatus_Disconnected {
		t.Fatalf("expected stored status disconnected after failed restart, got %q", got)
	}
	if got := lastStatusForBlock(blockId); got != "disconnected" {
		t.Fatalf("last published status = %q, stored status = %q", got, JobConnStatus_Disconnected)
	}
}
