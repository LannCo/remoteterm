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
	"time"

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

// fakeJobManager stands in for a remote job manager on the job:<id> route.
type fakeJobManager struct {
	lock            sync.Mutex
	jobId           string
	prepareHold     time.Duration
	prepareErr      error
	inflight        int
	maxInflight     int
	prepareCount    int
	startCount      int
	statusAtPrepare []string
}

func (f *fakeJobManager) WshServerImpl() {}

func (f *fakeJobManager) enterPrepare() {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.inflight++
	f.prepareCount++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	f.statusAtPrepare = append(f.statusAtPrepare, GetJobConnStatus(f.jobId))
}

func (f *fakeJobManager) exitPrepare() {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.inflight--
}

func (f *fakeJobManager) JobPrepareConnectCommand(ctx context.Context, data wshrpc.CommandJobPrepareConnectData) (*wshrpc.CommandJobConnectRtnData, error) {
	f.enterPrepare()
	defer f.exitPrepare()
	time.Sleep(f.prepareHold)
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	return &wshrpc.CommandJobConnectRtnData{Seq: data.Seq}, nil
}

func (f *fakeJobManager) JobStartStreamCommand(ctx context.Context, data wshrpc.CommandJobStartStreamData) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.startCount++
	return nil
}

type fakeJobManagerStats struct {
	maxInflight     int
	prepareCount    int
	startCount      int
	statusAtPrepare []string
}

func (f *fakeJobManager) stats() fakeJobManagerStats {
	f.lock.Lock()
	defer f.lock.Unlock()
	return fakeJobManagerStats{
		maxInflight:     f.maxInflight,
		prepareCount:    f.prepareCount,
		startCount:      f.startCount,
		statusAtPrepare: append([]string(nil), f.statusAtPrepare...),
	}
}

func registerFakeRoute(t *testing.T, impl wshutil.ServerImpl, routeId string) {
	t.Helper()
	rpc := wshutil.MakeWshRpc(wshrpc.RpcContext{}, impl, routeId)
	linkId, err := wshutil.DefaultRouter.RegisterTrustedLeaf(rpc, routeId)
	if err != nil {
		t.Fatalf("register route %s: %v", routeId, err)
	}
	t.Cleanup(func() { wshutil.DefaultRouter.UnregisterLink(linkId) })
}

func registerFakeJobManager(t *testing.T, fake *fakeJobManager) {
	t.Helper()
	registerFakeRoute(t, fake, wshutil.MakeJobRouteId(fake.jobId))
}

// TestReconnectEntrypointsSerializeRestartStreaming: ReconnectJob and the route-up
// ReconnectJobRoute use different singleflight groups; for the same job they must
// still never run restartStreaming concurrently (the second call would close the
// first's fresh reader while both send JobPrepareConnect to the job manager).
func TestReconnectEntrypointsSerializeRestartStreaming(t *testing.T) {
	initStoreFixture(t)
	ctx := context.Background()
	jobId, _ := makeRunningLocalJob(t)
	fake := &fakeJobManager{jobId: jobId, prepareHold: 300 * time.Millisecond, prepareErr: fmt.Errorf("fake prepare failure")}
	registerFakeJobManager(t, fake)
	SetJobConnStatus(jobId, JobConnStatus_Connected)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ReconnectJob(ctx, jobId, nil)
	}()
	go func() {
		defer wg.Done()
		ReconnectJobRoute(ctx, jobId, nil)
	}()
	wg.Wait()

	st := fake.stats()
	if st.prepareCount == 0 {
		t.Fatalf("expected at least one JobPrepareConnect")
	}
	if st.maxInflight > 1 {
		t.Fatalf("%d restartStreaming calls in flight at once for one job", st.maxInflight)
	}
}

// TestReconnectWaiterSkipsAfterHolderRestoresStream: a reconnect that waited on the
// job's reconnect lock must see the stream the previous holder just started and
// skip, rather than superseding it with a second JobPrepareConnect.
func TestReconnectWaiterSkipsAfterHolderRestoresStream(t *testing.T) {
	initStoreFixture(t)
	ctx := context.Background()
	jobId, _ := makeRunningLocalJob(t)
	fake := &fakeJobManager{jobId: jobId, prepareHold: 300 * time.Millisecond}
	registerFakeJobManager(t, fake)
	SetJobConnStatus(jobId, JobConnStatus_Connected)

	errs := make(chan error, 2)
	go func() { errs <- ReconnectJob(ctx, jobId, nil) }()
	go func() { errs <- ReconnectJobRoute(ctx, jobId, nil) }()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("reconnect returned error: %v", err)
		}
	}

	st := fake.stats()
	if st.prepareCount != 1 || st.startCount != 1 {
		t.Fatalf("expected one restart, got prepare=%d start=%d", st.prepareCount, st.startCount)
	}
	health, ok := jobStreamHealth.GetEx(jobId)
	if !hasActiveStream(health, ok) {
		t.Fatalf("expected an active stream after restart, health=%+v ok=%v", health, ok)
	}
	if got := GetJobConnStatus(jobId); got != JobConnStatus_Connected {
		t.Fatalf("expected job Connected, got %q", got)
	}
}

// TestStartJobFailureReleasesStream: a StartJob that fails after registering its
// output stream must not leave the reader registered or stream health claiming an
// active output loop for a job that never started one.
func TestStartJobFailureReleasesStream(t *testing.T) {
	initStoreFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := "bash-" + uuid.New().String()

	// No conn:local route is registered, so RemoteStartJobCommand fails.
	if _, err := StartJob(ctx, StartJobParams{ConnName: "local", JobKind: JobKind_Shell, Cmd: cmd}); err == nil {
		t.Fatalf("expected StartJob to fail with no connection route")
	}

	jobs, err := rtstore.DBGetAllObjsByType[*remotetermobj.Job](ctx, remotetermobj.OType_Job)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var jobId string
	for _, job := range jobs {
		if job.Cmd == cmd {
			jobId = job.OID
		}
	}
	if jobId == "" {
		t.Fatalf("failed job not found in store")
	}
	health, healthOk := jobStreamHealth.GetEx(jobId)
	if hasActiveStream(health, healthOk) {
		t.Errorf("stream health still active after failed start: %+v", health)
	}
	if _, ok := jobReaders.GetEx(jobId); ok {
		t.Errorf("reader still registered after failed start")
	}
	if _, ok := jobStreamIds.GetEx(jobId); ok {
		t.Errorf("stream id still registered after failed start")
	}
}
