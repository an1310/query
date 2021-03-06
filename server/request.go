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
	"net/http"
	"runtime"
	"sync"
	"time"

	atomic "github.com/couchbase/go-couchbase/platform"
	"github.com/couchbase/query/datastore"
	"github.com/couchbase/query/errors"
	"github.com/couchbase/query/execution"
	"github.com/couchbase/query/plan"
	"github.com/couchbase/query/timestamp"
	"github.com/couchbase/query/util"
	"github.com/couchbase/query/value"
)

type RequestChannel chan Request

const _ERROR_CAP = 4

type State string

const (
	RUNNING   State = "running"
	SUCCESS   State = "success"
	ERRORS    State = "errors"
	COMPLETED State = "completed"
	STOPPED   State = "stopped"
	TIMEOUT   State = "timeout"
	CLOSED    State = "closed"
	FATAL     State = "fatal"
)

type Request interface {
	Id() RequestID
	ClientID() ClientContextID
	Statement() string
	Prepared() *plan.Prepared
	SetPrepared(prepared *plan.Prepared)
	NamedArgs() map[string]value.Value
	PositionalArgs() value.Values
	Namespace() string
	Timeout() time.Duration
	MaxParallelism() int
	Readonly() value.Tristate
	Metrics() value.Tristate
	Signature() value.Tristate
	Pretty() value.Tristate
	Controls() value.Tristate
	Profile() Profile
	ScanConsistency() datastore.ScanConsistency
	ScanVectorSource() timestamp.ScanVectorSource
	RequestTime() time.Time
	ServiceTime() time.Time
	Output() execution.Output
	CloseNotify() chan bool
	Servicing()
	Fail(err errors.Error)
	Execute(server *Server, signature value.Value, notifyStop chan bool)
	Failed(server *Server)
	Expire(state State, timeout time.Duration)
	SortCount() uint64
	State() State
	Halted() bool
	Credentials() datastore.Credentials
	SetTimings(o execution.Operator)
	GetTimings() execution.Operator
	OriginalHttpRequest() *http.Request
}

type RequestID interface {
	String() string
}

type ClientContextID interface {
	IsValid() bool
	String() string
}

type ScanConsistency int

const (
	NOT_BOUNDED ScanConsistency = iota
	REQUEST_PLUS
	STATEMENT_PLUS
	AT_PLUS
	UNDEFINED_CONSISTENCY
)

type ScanConfiguration interface {
	ScanConsistency() datastore.ScanConsistency
	ScanWait() time.Duration
	ScanVectorSource() timestamp.ScanVectorSource
}

// API for tracking active requests
type ActiveRequests interface {
	Put(Request) errors.Error
	Get(string, func(Request)) errors.Error
	Delete(string, bool) bool
	Count() (int, errors.Error)
	ForEach(func(string, Request))
}

var actives ActiveRequests

func ActiveRequestsCount() (int, errors.Error) {
	return actives.Count()
}

func ActiveRequestsDelete(id string) bool {
	return actives.Delete(id, true)
}

func ActiveRequestsGet(id string, f func(Request)) errors.Error {
	return actives.Get(id, f)
}

func ActiveRequestsForEach(f func(string, Request)) {
	actives.ForEach(f)
}

func SetActives(ar ActiveRequests) {
	actives = ar
}

type BaseRequest struct {
	// Aligned ints need to be declared right at the top
	// of the struct to avoid alignment issues on x86 platforms
	mutationCount atomic.AlignedUint64
	sortCount     atomic.AlignedUint64
	phaseStats    [execution.PHASES]phaseStat

	sync.RWMutex
	id             *requestIDImpl
	client_id      *clientContextIDImpl
	statement      string
	prepared       *plan.Prepared
	namedArgs      map[string]value.Value
	positionalArgs value.Values
	namespace      string
	timeout        time.Duration
	maxParallelism int
	readonly       value.Tristate
	signature      value.Tristate
	metrics        value.Tristate
	pretty         value.Tristate
	consistency    ScanConfiguration
	credentials    datastore.Credentials
	requestTime    time.Time
	serviceTime    time.Time
	state          State
	results        value.ValueChannel
	errors         errors.ErrorChannel
	warnings       errors.ErrorChannel
	closeNotify    chan bool // implement http.CloseNotifier
	stopNotify     chan bool // notified when request execution stops
	stopResult     chan bool // stop consuming results
	stopExecute    chan bool // stop executing request
	timings        execution.Operator
	controls       value.Tristate
	profile        Profile
}

type requestIDImpl struct {
	id string
}

type phaseStat struct {
	count     atomic.AlignedUint64
	operators atomic.AlignedUint64
	duration  atomic.AlignedUint64
}

// requestIDImpl implements the RequestID interface
func (r *requestIDImpl) String() string {
	return r.id
}

type clientContextIDImpl struct {
	id string
}

func (this *clientContextIDImpl) IsValid() bool {
	return len(this.id) > 0
}

func (this *clientContextIDImpl) String() string {
	return this.id
}

func newClientContextIDImpl(id string) *clientContextIDImpl {
	return &clientContextIDImpl{id: id}
}

func NewBaseRequest(statement string, prepared *plan.Prepared, namedArgs map[string]value.Value, positionalArgs value.Values,
	namespace string, maxParallelism int, readonly, metrics, signature, pretty value.Tristate, consistency ScanConfiguration,
	client_id string, creds datastore.Credentials) *BaseRequest {
	rv := &BaseRequest{
		statement:      statement,
		prepared:       prepared,
		namedArgs:      namedArgs,
		positionalArgs: positionalArgs,
		namespace:      namespace,
		maxParallelism: maxParallelism,
		readonly:       readonly,
		signature:      signature,
		metrics:        metrics,
		pretty:         pretty,
		consistency:    consistency,
		credentials:    creds,
		requestTime:    time.Now(),
		serviceTime:    time.Now(),
		state:          RUNNING,
		errors:         make(errors.ErrorChannel, _ERROR_CAP),
		warnings:       make(errors.ErrorChannel, _ERROR_CAP),
		closeNotify:    make(chan bool, 1),
		stopResult:     make(chan bool, 1),
		stopExecute:    make(chan bool, 1),
		profile:        ProfUnset,
		controls:       value.NONE,
	}

	if maxParallelism <= 0 {
		maxParallelism = runtime.NumCPU()
	}

	rv.results = make(value.ValueChannel, maxParallelism)

	uuid, _ := util.UUID()
	rv.id = &requestIDImpl{id: uuid}
	rv.client_id = newClientContextIDImpl(client_id)
	return rv
}

func (this *BaseRequest) SetTimeout(request Request, timeout time.Duration) {
	this.timeout = timeout

	// Apply request timeout
	if timeout > 0 {
		time.AfterFunc(timeout, func() { request.Expire(TIMEOUT, timeout) })
	}
}

func (this *BaseRequest) Id() RequestID {
	return this.id
}

func (this *BaseRequest) ClientID() ClientContextID {
	return this.client_id
}

func (this *BaseRequest) Statement() string {
	return this.statement
}

func (this *BaseRequest) Prepared() *plan.Prepared {
	return this.prepared
}

func (this *BaseRequest) NamedArgs() map[string]value.Value {
	return this.namedArgs
}

func (this *BaseRequest) PositionalArgs() value.Values {
	return this.positionalArgs
}

func (this *BaseRequest) Namespace() string {
	return this.namespace
}

func (this *BaseRequest) Timeout() time.Duration {
	return this.timeout
}

func (this *BaseRequest) MaxParallelism() int {
	return this.maxParallelism
}

func (this *BaseRequest) Readonly() value.Tristate {
	return this.readonly
}

func (this *BaseRequest) Signature() value.Tristate {
	return this.signature
}

func (this *BaseRequest) Metrics() value.Tristate {
	return this.metrics
}

func (this *BaseRequest) Pretty() value.Tristate {
	return this.pretty
}

func (this *BaseRequest) ScanConsistency() datastore.ScanConsistency {
	if this.consistency == nil {
		return datastore.UNBOUNDED
	}
	return this.consistency.ScanConsistency()
}

func (this *BaseRequest) ScanVectorSource() timestamp.ScanVectorSource {
	if this.consistency == nil {
		return nil
	}
	return this.consistency.ScanVectorSource()
}

func (this *BaseRequest) RequestTime() time.Time {
	return this.requestTime
}

func (this *BaseRequest) ServiceTime() time.Time {
	return this.serviceTime
}

func (this *BaseRequest) SetPrepared(prepared *plan.Prepared) {
	this.Lock()
	defer this.Unlock()
	this.prepared = prepared
}

func (this *BaseRequest) SetState(state State) {

	// Once we transition to TIMEOUT or CLOSE, we don't transition
	// to STOPPED or COMPLETED to allow the request to close
	// gracefully on timeout or network errors and report the
	// right state
	if (this.state == TIMEOUT || this.state == CLOSED) &&
		(state == STOPPED || state == COMPLETED) {
		return
	}

	this.Lock()
	defer this.Unlock()
	this.state = state
}

func (this *BaseRequest) State() State {
	this.RLock()
	defer this.RUnlock()
	return this.state
}

func (this *BaseRequest) Halted() bool {

	// we purposly do not take the lock
	// as this is used repeatedly in Execution()
	// if we mistakenly report the State as RUNNING,
	// we'll catch the right state in other places...
	return this.state != RUNNING
}

func (this *BaseRequest) Credentials() datastore.Credentials {
	return this.credentials
}

func (this *BaseRequest) CloseNotify() chan bool {
	return this.closeNotify
}

func (this *BaseRequest) Servicing() {
	this.serviceTime = time.Now()
}

func (this *BaseRequest) Result(item value.Value) bool {
	select {
	case <-this.stopResult:
		return false
	default:
	}

	select {
	case this.results <- item:
		return true
	case <-this.stopResult:
		return false
	}
}

func (this *BaseRequest) CloseResults() {
	close(this.results)
}

func (this *BaseRequest) Fatal(err errors.Error) {
	defer this.Stop(FATAL)

	this.Error(err)
}

func (this *BaseRequest) Error(err errors.Error) {
	select {
	case this.errors <- err:
	default:
	}
}

func (this *BaseRequest) Warning(wrn errors.Error) {
	select {
	case this.warnings <- wrn:
	default:
	}
}

func (this *BaseRequest) AddMutationCount(i uint64) {
	atomic.AddUint64(&this.mutationCount, i)
}

func (this *BaseRequest) MutationCount() uint64 {
	return atomic.LoadUint64(&this.mutationCount)
}

func (this *BaseRequest) SetSortCount(i uint64) {
	atomic.StoreUint64(&this.sortCount, i)
}

func (this *BaseRequest) SortCount() uint64 {
	return atomic.LoadUint64(&this.sortCount)
}

func (this *BaseRequest) AddPhaseCount(p execution.Phases, c uint64) {
	atomic.AddUint64(&this.phaseStats[p].count, c)
}

func (this *BaseRequest) AddPhaseOperator(p execution.Phases) {
	atomic.AddUint64(&this.phaseStats[p].operators, 1)
}

func (this *BaseRequest) FmtPhaseCounts() map[string]interface{} {
	var p map[string]interface{} = nil

	// Use simple iteration rather than a range clause to avoid a spurious
	// data race report. MB-20692
	nr := len(this.phaseStats)
	for i := 0; i < nr; i++ {
		count := atomic.LoadUint64(&this.phaseStats[i].count)
		if count > 0 {
			if p == nil {
				p = make(map[string]interface{},
					execution.PHASES)
			}
			p[execution.Phases(i).String()] = count
		}
	}
	return p
}

func (this *BaseRequest) FmtPhaseOperators() map[string]interface{} {
	var p map[string]interface{} = nil

	// Use simple iteration rather than a range clause to avoid a spurious
	// data race report. MB-20692
	nr := len(this.phaseStats)
	for i := 0; i < nr; i++ {
		operators := atomic.LoadUint64(&this.phaseStats[i].operators)
		if operators > 0 {
			if p == nil {
				p = make(map[string]interface{},
					execution.PHASES)
			}
			p[execution.Phases(i).String()] = operators
		}
	}
	return p
}

func (this *BaseRequest) AddPhaseTime(phase execution.Phases, duration time.Duration) {
	atomic.AddUint64(&(this.phaseStats[phase].duration), uint64(duration))
}

func (this *BaseRequest) FmtPhaseTimes() map[string]interface{} {
	var p map[string]interface{} = nil

	nr := len(this.phaseStats)
	for i := 0; i < nr; i++ {
		duration := atomic.LoadUint64(&this.phaseStats[i].duration)
		if duration > 0 {
			if p == nil {
				p = make(map[string]interface{},
					execution.PHASES)
			}
			p[execution.Phases(i).String()] = time.Duration(duration).String()
		}
	}
	return p
}

func (this *BaseRequest) SetTimings(o execution.Operator) {
	this.timings = o
}

func (this *BaseRequest) GetTimings() execution.Operator {
	return this.timings
}

func (this *BaseRequest) SetControls(c value.Tristate) {
	this.controls = c
}

func (this *BaseRequest) Controls() value.Tristate {
	return this.controls
}

func (this *BaseRequest) SetProfile(p Profile) {
	this.profile = p
}

func (this *BaseRequest) Profile() Profile {
	return this.profile
}

func (this *BaseRequest) Results() value.ValueChannel {
	return this.results
}

func (this *BaseRequest) Errors() errors.ErrorChannel {
	return this.errors
}

func (this *BaseRequest) Warnings() errors.ErrorChannel {
	return this.warnings
}

func (this *BaseRequest) NotifyStop(ch chan bool) {
	this.Lock()
	defer this.Unlock()
	this.stopNotify = ch
}

func (this *BaseRequest) StopNotify() chan bool {
	this.RLock()
	defer this.RUnlock()
	return this.stopNotify
}

func (this *BaseRequest) StopExecute() chan bool {
	return this.stopExecute
}

func (this *BaseRequest) Stop(state State) {
	defer sendStop(this.StopNotify())
	defer sendStop(this.stopResult)
	defer sendStop(this.stopExecute)

	this.SetState(state)
}

func (this *BaseRequest) Close() {
	sendStop(this.closeNotify)
}

// this logs the request if needed and takes any other action required to
// put this request to rest
func (this *BaseRequest) CompleteRequest(requestTime time.Duration, serviceTime time.Duration,
	resultCount int, resultSize int, errorCount int, server *Server) {

	LogRequest(requestTime, serviceTime, resultCount,
		resultSize, errorCount, this, server)

	// Request Profiling - signal that request has completed and
	// resources can be pooled / released as necessary
	if this.timings != nil {
		this.timings.Done()
		this.timings = nil
	}
}

func sendStop(ch chan bool) {
	select {
	case ch <- false:
	default:
	}
}
