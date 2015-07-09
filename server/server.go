//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package server

import (
	"encoding/json"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	atomic "github.com/couchbase/go-couchbase/platform"
	"github.com/couchbase/query/accounting"
	"github.com/couchbase/query/clustering"
	"github.com/couchbase/query/datastore"
	"github.com/couchbase/query/datastore/system"
	"github.com/couchbase/query/errors"
	"github.com/couchbase/query/execution"
	"github.com/couchbase/query/logging"
	"github.com/couchbase/query/parser/n1ql"
	"github.com/couchbase/query/plan"
	"github.com/couchbase/query/planner"
	"github.com/couchbase/query/value"
)

type Server struct {
	// due to alignment issues on x86 platforms these atomic
	// variables need to right at the beginning of the structure
	servicerCount  atomic.AlignedInt64
	maxParallelism atomic.AlignedInt64
	keepAlive      atomic.AlignedInt64
	requestSize    atomic.AlignedInt64

	sync.RWMutex
	datastore   datastore.Datastore
	systemstore datastore.Datastore
	configstore clustering.ConfigurationStore
	acctstore   accounting.AccountingStore
	namespace   string
	readonly    bool
	channel     RequestChannel
	done        chan bool
	timeout     time.Duration
	signature   bool
	metrics     bool
	wg          sync.WaitGroup
	memprofile  string
	cpuprofile  string
}

// Default Keep Alive Length

const KEEP_ALIVE_DEFAULT = 1024 * 16

func NewServer(store datastore.Datastore, config clustering.ConfigurationStore,
	acctng accounting.AccountingStore, namespace string, readonly bool,
	channel RequestChannel, servicerCount, maxParallelism int, timeout time.Duration,
	signature, metrics bool) (*Server, errors.Error) {
	rv := &Server{
		datastore:   store,
		configstore: config,
		acctstore:   acctng,
		namespace:   namespace,
		readonly:    readonly,
		channel:     channel,
		signature:   signature,
		timeout:     timeout,
		metrics:     metrics,
		done:        make(chan bool),
	}

	// special case handling for the atomic specfic stuff
	atomic.StoreInt64(&rv.servicerCount, int64(servicerCount))

	store.SetLogLevel(logging.LogLevel())
	rv.SetMaxParallelism(maxParallelism)

	sys, err := system.NewDatastore(store)
	if err != nil {
		return nil, err
	}

	rv.systemstore = sys
	return rv, nil
}

func (this *Server) Datastore() datastore.Datastore {
	return this.datastore
}

func (this *Server) ConfigurationStore() clustering.ConfigurationStore {
	return this.configstore
}

func (this *Server) AccountingStore() accounting.AccountingStore {
	return this.acctstore
}

func (this *Server) Channel() RequestChannel {
	return this.channel
}

func (this *Server) Signature() bool {
	return this.signature
}

func (this *Server) Metrics() bool {
	return this.metrics
}

func (this *Server) KeepAlive() int {
	return int(atomic.LoadInt64(&this.keepAlive))
}

func (this *Server) SetKeepAlive(keepAlive int) {
	if keepAlive <= 0 {
		keepAlive = KEEP_ALIVE_DEFAULT
	}
	atomic.StoreInt64(&this.keepAlive, int64(keepAlive))
}

func (this *Server) MaxParallelism() int {
	return int(atomic.LoadInt64(&this.maxParallelism))
}

func (this *Server) SetMaxParallelism(maxParallelism int) {
	if maxParallelism <= 0 {
		maxParallelism = runtime.NumCPU()
	}
	atomic.StoreInt64(&this.maxParallelism, int64(maxParallelism))
}

func (this *Server) Memprofile() string {
	this.RLock()
	defer this.RUnlock()
	return this.memprofile
}

func (this *Server) SetMemprofile(memprofile string) {
	this.Lock()
	defer this.Unlock()
	this.memprofile = memprofile
}

func (this *Server) Cpuprofile() string {
	this.RLock()
	defer this.RUnlock()
	return this.cpuprofile
}

func (this *Server) SetCpuprofile(cpuprofile string) {
	this.Lock()
	defer this.Unlock()
	this.cpuprofile = cpuprofile
	if this.cpuprofile == "" {
		return
	}
	f, err := os.Create(this.cpuprofile)
	if err != nil {
		logging.Errorp("Cannot start cpu profiler", logging.Pair{"error", err})
		this.cpuprofile = ""
	} else {
		pprof.StartCPUProfile(f)
	}
}

func (this *Server) PipelineCap() int {
	return int(execution.GetPipelineCap())
}

func (this *Server) SetPipelineCap(pipeline_cap int) {
	execution.SetPipelineCap(pipeline_cap)
}

func (this *Server) Debug() bool {
	return logging.LogLevel() == logging.DEBUG
}

func (this *Server) SetDebug(debug bool) {
	if debug {
		logging.SetLevel(logging.DEBUG)
	} else {
		logging.SetLevel(logging.INFO)
	}
}

const (
	MAX_REQUEST_SIZE = 64 * (1 << 20)
)

func (this *Server) RequestSizeCap() int {
	return int(atomic.LoadInt64(&this.requestSize))
}

func (this *Server) SetRequestSizeCap(requestSize int) {
	if requestSize <= 0 {
		requestSize = math.MaxInt32
	}
	atomic.StoreInt64(&this.requestSize, int64(requestSize))
}

func (this *Server) ScanCap() int {
	return int(datastore.GetScanCap())
}

func (this *Server) SetScanCap(size int) {
	datastore.SetScanCap(int64(size))
}

func (this *Server) Servicers() int {
	return int(atomic.LoadInt64(&this.servicerCount))
}

func (this *Server) SetServicers(servicerCount int) {
	this.Lock()
	defer this.Unlock()
	// Stop the current set of servicers
	close(this.done)
	logging.Infop("SetServicers - waiting for current servicers to finish")
	this.wg.Wait()
	// Set servicer count and recreate servicer channel
	atomic.StoreInt64(&this.servicerCount, int64(servicerCount))
	logging.Infop("SetServicers - starting new servicers")
	// Start new set of servicers
	this.done = make(chan bool)
	go this.Serve()
}

func (this *Server) Timeout() time.Duration {
	return this.timeout
}

func (this *Server) SetTimeout(timeout time.Duration) {
	this.timeout = timeout
}

func (this *Server) Serve() {
	// Use a threading model. Do not spawn a separate
	// goroutine for each request, as that would be
	// unbounded and could degrade performance of already
	// executing queries.
	servicers := this.Servicers()
	this.wg.Add(servicers)
	for i := 0; i < servicers; i++ {
		go this.doServe()
	}
}

func (this *Server) doServe() {
	defer this.wg.Done()
	ok := true
	for ok {
		select {
		case request := <-this.channel:
			this.serviceRequest(request)
		case <-this.done:
			ok = false
		}
	}
}

func (this *Server) serviceRequest(request Request) {
	defer func() {
		err := recover()
		if err != nil {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, false)
			s := string(buf[0:n])
			logging.Severep("", logging.Pair{"panic", err},
				logging.Pair{"stack", s})
			os.Stderr.WriteString(s)
			os.Stderr.Sync()
		}
	}()

	request.Servicing()

	namespace := request.Namespace()
	if namespace == "" {
		namespace = this.namespace
	}

	prepared, err := this.getPrepared(request, namespace)
	if err != nil {
		request.Fail(err)
	}

	if (this.readonly || value.ToBool(request.Readonly())) &&
		(prepared != nil && !prepared.Readonly()) {
		request.Fail(errors.NewServiceErrorReadonly("The server or request is read-only" +
			" and cannot accept this write statement."))
	}

	if request.State() == FATAL {
		request.Failed(this)
		return
	}

	maxParallelism := request.MaxParallelism()
	if maxParallelism <= 0 {
		maxParallelism = this.MaxParallelism()
	}

	context := execution.NewContext(request.Id().String(), this.datastore, this.systemstore, namespace,
		this.readonly, maxParallelism, request.NamedArgs(), request.PositionalArgs(),
		request.Credentials(), request.ScanConsistency(), request.ScanVector(), request.Output())

	build := time.Now()
	operator, er := execution.Build(prepared, context)
	if er != nil {
		request.Fail(errors.NewError(err, ""))
	}

	if logging.LogLevel() >= logging.TRACE {
		request.Output().AddPhaseTime("instantiate", time.Since(build))
	}

	if request.State() == FATAL {
		request.Failed(this)
		return
	}

	// Apply server execution timeout
	if this.Timeout() > 0 {
		timer := time.AfterFunc(this.Timeout(), func() { request.Expire() })
		defer timer.Stop()
	}

	go request.Execute(this, prepared.Signature(), operator.StopChannel())

	run := time.Now()
	operator.RunOnce(context, nil)

	if logging.LogLevel() >= logging.TRACE {
		request.Output().AddPhaseTime("run", time.Since(run))
		logPhases(request)
	}
}

func (this *Server) getPrepared(request Request, namespace string) (*plan.Prepared, errors.Error) {
	prepared := request.Prepared()
	if prepared == nil {
		parse := time.Now()
		stmt, err := n1ql.ParseStatement(request.Statement())
		if err != nil {
			return nil, errors.NewParseSyntaxError(err, "")
		}

		prep := time.Now()
		prepared, err = planner.BuildPrepared(stmt, this.datastore, this.systemstore, namespace, false)
		if err != nil {
			return nil, errors.NewPlanError(err, "")
		}

		if logging.LogLevel() >= logging.TRACE {
			request.Output().AddPhaseTime("plan", time.Since(prep))
			request.Output().AddPhaseTime("parse", prep.Sub(parse))
		}
	}

	if logging.LogLevel() >= logging.DEBUG {
		// log EXPLAIN for the request
		logExplain(prepared)
	}

	return prepared, nil
}

func logExplain(prepared *plan.Prepared) {
	var pl plan.Operator = prepared
	explain, err := json.MarshalIndent(pl, "", "    ")
	if err != nil {
		logging.Tracep("Error logging explain", logging.Pair{"error", err})
		return
	}

	logging.Tracep("Explain ", logging.Pair{"explain", string(explain)})
}

func logPhases(request Request) {
	phaseTimes := request.Output().PhaseTimes()
	if len(phaseTimes) == 0 {
		return
	}

	pairs := make([]logging.Pair, 0, len(phaseTimes)+1)
	pairs = append(pairs, logging.Pair{"_id", request.Id()})
	for k, v := range phaseTimes {
		pairs = append(pairs, logging.Pair{k, v})
	}

	logging.Tracep("Phase aggregates", pairs...)
}
