// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jobcontroller

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LannCo/remoteterm/pkg/filestore"
	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/remotetermjwt"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/util/shellutil"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
	"github.com/LannCo/remoteterm/pkg/wshrpc/wshclient"
	"github.com/google/uuid"
)

// startJobCapture stands in for the local connserver: it records the start request and
// fails it, so StartJob stops right after the log line and DB insert under test.
type startJobCapture struct {
	lock sync.Mutex
	data *wshrpc.CommandRemoteStartJobData
}

func (f *startJobCapture) WshServerImpl() {}

func (f *startJobCapture) RemoteStartJobCommand(ctx context.Context, data wshrpc.CommandRemoteStartJobData) (*wshrpc.CommandStartJobRtnData, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.data = &data
	return nil, fmt.Errorf("start rejected by test")
}

func (f *startJobCapture) captured() *wshrpc.CommandRemoteStartJobData {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.data
}

func TestStartJobDoesNotPersistOrLogSecrets(t *testing.T) {
	initStoreFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	capture := &startJobCapture{}
	registerFakeRoute(t, capture, "conn:local")

	var logBuf syncBuffer
	prevLogOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(prevLogOutput) })

	secrets := map[string]string{
		remotetermbase.WaveSwapTokenVarName:      "c3dhcHRva2VuLXNlY3JldC12YWx1ZS0x",
		remotetermbase.WaveJwtTokenVarName:       "eyJhbGciOiJFZERTQSJ9.eyJzZWNyZXQiOiJqd3QifQ.c2lnMQ",
		remotetermbase.LegacyWaveJwtTokenVarName: "eyJhbGciOiJFZERTQSJ9.eyJzZWNyZXQiOiJsZWdhY3kifQ.c2lnMg",
	}
	env := map[string]string{"TERM": "xterm-256color"}
	for k, v := range secrets {
		env[k] = v
	}
	cmd := "bash-" + uuid.New().String()
	if _, err := StartJob(ctx, StartJobParams{ConnName: "local", JobKind: JobKind_Shell, Cmd: cmd, Env: env}); err == nil {
		t.Fatalf("expected StartJob to fail on the rejecting connserver")
	}

	sent := capture.captured()
	if sent == nil {
		t.Fatalf("RemoteStartJobCommand was not called")
	}
	for k, v := range secrets {
		if sent.Env[k] != v {
			t.Errorf("remote start env %s = %q, want the real secret", k, sent.Env[k])
		}
	}

	jobs, err := rtstore.DBGetAllObjsByType[*remotetermobj.Job](ctx, remotetermobj.OType_Job)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var job *remotetermobj.Job
	for _, j := range jobs {
		if j.Cmd == cmd {
			job = j
		}
	}
	if job == nil {
		t.Fatalf("job not found in store")
	}
	if job.CmdEnv["TERM"] != "xterm-256color" {
		t.Errorf("non-secret env not persisted: %v", job.CmdEnv)
	}
	logged := logBuf.String()
	for k, v := range secrets {
		if strings.Contains(fmt.Sprintf("%v", job.CmdEnv), v) {
			t.Errorf("DB row CmdEnv contains %s value", k)
		}
		if strings.Contains(logged, v) {
			t.Errorf("log output contains %s value", k)
		}
	}
	if !strings.Contains(logged, shellutil.RedactSecret(secrets[remotetermbase.WaveJwtTokenVarName])) {
		t.Errorf("expected redacted JWT marker in log, got:\n%s", logged)
	}

	claims, err := remotetermjwt.ValidateAndExtract(sent.MainServerJwtToken)
	if err != nil {
		t.Fatalf("job access token invalid: %v", err)
	}
	if claims.ExpiresAt == nil {
		t.Fatalf("job access token has no expiry")
	}
	if ttl := time.Until(claims.ExpiresAt.Time); ttl > JobAccessTokenExpiry || ttl < JobAccessTokenExpiry-time.Minute {
		t.Errorf("job access token ttl = %v, want about %v", ttl, JobAccessTokenExpiry)
	}
}

// TestRetirePrevStreamPreservesOutputOrder pauses the old output loop after it has taken the
// first chunk from the reader but before it appends it (the drain-progress status event fires
// in that window), then retires the stream. Draining the buffer before the loop has exited
// would append the later bytes ahead of that chunk.
func TestRetirePrevStreamPreservesOutputOrder(t *testing.T) {
	initStoreFixture(t)
	ctx := context.Background()
	jobId, blockId := makeRunningLocalJob(t)
	fileOpts := wshrpc.FileOpts{MaxSize: 10 * 1024 * 1024, Circular: true}
	for _, zone := range []string{jobId, blockId} {
		if err := filestore.WFS.MakeFile(ctx, zone, JobOutputFileName, wshrpc.FileMeta{}, fileOpts); err != nil {
			t.Fatalf("make file: %v", err)
		}
	}
	broker := wshclient.GetBareRpcClient().StreamBroker
	reader, meta := broker.CreateStreamReader(wshclient.GetBareRpcClientRouteId(), "job:"+jobId, DefaultStreamRwnd)
	registerNewJobStream(jobId, reader, meta.Id)

	var want bytes.Buffer
	for p := 0; p < 60; p++ {
		chunk := []byte(strings.Repeat(fmt.Sprintf("<%04d>", p), 200)[:1000])
		reader.RecvData(wshrpc.CommandStreamData{
			Id:     meta.Id,
			Seq:    int64(want.Len()),
			Data64: base64.StdEncoding.EncodeToString(chunk),
		})
		want.Write(chunk)
	}

	paused := make(chan struct{})
	release := make(chan struct{})
	var pauseOnce sync.Once
	statusRecorder.setHook(func(data *wshrpc.BlockJobStatusData) {
		if data.BlockId != blockId {
			return
		}
		pauseOnce.Do(func() {
			close(paused)
			<-release
		})
	})
	t.Cleanup(func() { statusRecorder.setHook(nil) })
	jobDrainProgress.Set(jobId, drainProgressInfo{active: true, totalBytes: 1, remainingBytes: 1})

	loopDone := make(chan struct{})
	go func() {
		defer func() {
			panichandler.PanicHandler("test:runOutputLoop", recover())
			close(loopDone)
		}()
		runOutputLoop(ctx, jobId, meta.Id, reader)
	}()
	select {
	case <-paused:
	case <-time.After(2 * time.Second):
		t.Fatalf("output loop never reached the drain-progress event")
	}

	retireDone := make(chan struct{})
	go func() {
		defer close(retireDone)
		retirePrevStream(ctx, jobId)
	}()
	// Well inside waitForStreamLoopExit's 1s bound, and ample time for an early drain.
	time.Sleep(200 * time.Millisecond)
	close(release)

	for name, done := range map[string]chan struct{}{"output loop": loopDone, "retirePrevStream": retireDone} {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
	_, got, err := filestore.WFS.ReadFile(ctx, jobId, JobOutputFileName)
	if err != nil {
		t.Fatalf("read job file: %v", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("job file does not match stream order (got %d bytes, want %d); starts %q",
			len(got), want.Len(), got[:min(len(got), 24)])
	}
	if job, _ := rtstore.DBGet[*remotetermobj.Job](ctx, jobId); job != nil && job.StreamDone {
		t.Fatalf("retiring the stream took the error path and marked it done")
	}
}

// syncBuffer is a log sink safe for the goroutines StartJob leaves logging behind it.
type syncBuffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.String()
}
