//  Copyright (c) 2016 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package system

import (
	"github.com/couchbase/query/datastore"
	"github.com/couchbase/query/errors"
	"github.com/couchbase/query/expression"
	"github.com/couchbase/query/timestamp"
	"github.com/couchbase/query/value"
)

type myUserInfoKeyspace struct {
	namespace *namespace
	name      string
	indexer   datastore.Indexer
	cache     *userInfoCache
}

func (b *myUserInfoKeyspace) Release() {
}

func (b *myUserInfoKeyspace) NamespaceId() string {
	return b.namespace.Id()
}

func (b *myUserInfoKeyspace) Id() string {
	return b.Name()
}

func (b *myUserInfoKeyspace) Name() string {
	return b.name
}

func (b *myUserInfoKeyspace) Count() (int64, errors.Error) {
	v, err := b.cache.getNumUsers()
	return int64(v), err
}

func (b *myUserInfoKeyspace) Indexer(name datastore.IndexType) (datastore.Indexer, errors.Error) {
	return b.indexer, nil
}

func (b *myUserInfoKeyspace) Indexers() ([]datastore.Indexer, errors.Error) {
	return []datastore.Indexer{b.indexer}, nil
}

func (b *myUserInfoKeyspace) Fetch(keys []string) ([]value.AnnotatedPair, []errors.Error) {
	vals, errs := b.cache.fetch(keys)
	return vals, errs
}

func (b *myUserInfoKeyspace) Insert(inserts []value.Pair) ([]value.Pair, errors.Error) {
	return nil, errors.NewSystemNotImplementedError(nil, "")
}

func (b *myUserInfoKeyspace) Update(updates []value.Pair) ([]value.Pair, errors.Error) {
	return nil, errors.NewSystemNotImplementedError(nil, "")
}

func (b *myUserInfoKeyspace) Upsert(upserts []value.Pair) ([]value.Pair, errors.Error) {
	return nil, errors.NewSystemNotImplementedError(nil, "")
}

func (b *myUserInfoKeyspace) Delete(deletes []string) ([]string, errors.Error) {
	return nil, errors.NewSystemNotImplementedError(nil, "")
}

func newMyUserInfoKeyspace(p *namespace) (*myUserInfoKeyspace, errors.Error) {
	b := new(myUserInfoKeyspace)
	b.namespace = p
	b.name = KEYSPACE_NAME_MY_USER_INFO

	primary := &myUserInfoIndex{name: "#primary", keyspace: b}
	b.indexer = newSystemIndexer(b, primary)

	b.cache = newUserInfoCache(p.store)

	return b, nil
}

type myUserInfoIndex struct {
	name     string
	keyspace *myUserInfoKeyspace
}

func (pi *myUserInfoIndex) KeyspaceId() string {
	return pi.keyspace.Id()
}

func (pi *myUserInfoIndex) Id() string {
	return pi.Name()
}

func (pi *myUserInfoIndex) Name() string {
	return pi.name
}

func (pi *myUserInfoIndex) Type() datastore.IndexType {
	return datastore.DEFAULT
}

func (pi *myUserInfoIndex) SeekKey() expression.Expressions {
	return nil
}

func (pi *myUserInfoIndex) RangeKey() expression.Expressions {
	return nil
}

func (pi *myUserInfoIndex) Condition() expression.Expression {
	return nil
}

func (pi *myUserInfoIndex) IsPrimary() bool {
	return true
}

func (pi *myUserInfoIndex) State() (state datastore.IndexState, msg string, err errors.Error) {
	return datastore.ONLINE, "", nil
}

func (pi *myUserInfoIndex) Statistics(requestId string, span *datastore.Span) (
	datastore.Statistics, errors.Error) {
	return nil, nil
}

func (pi *myUserInfoIndex) Drop(requestId string) errors.Error {
	return errors.NewSystemIdxNoDropError(nil, "")
}

func (pi *myUserInfoIndex) Scan(requestId string, span *datastore.Span, distinct bool, limit int64,
	cons datastore.ScanConsistency, vector timestamp.Vector, conn *datastore.IndexConnection) {

	noUsers := make(datastore.AuthenticatedUsers, 0)
	pi.ScanEntriesForUsers(requestId, limit, cons, vector, noUsers, conn)
}

func (pi *myUserInfoIndex) ScanEntries(requestId string, limit int64, cons datastore.ScanConsistency,
	vector timestamp.Vector, conn *datastore.IndexConnection) {

	noUsers := make(datastore.AuthenticatedUsers, 0)
	pi.ScanEntriesForUsers(requestId, limit, cons, vector, noUsers, conn)
}

func (pi *myUserInfoIndex) ScanEntriesForUsers(requestId string, limit int64, cons datastore.ScanConsistency,
	vector timestamp.Vector, au datastore.AuthenticatedUsers, conn *datastore.IndexConnection) {
	defer close(conn.EntryChannel())

	f := func(userId string) bool {
		for _, v := range au {
			if v == userId {
				return true
			}
		}
		return false
	}

	pi.keyspace.cache.scanEntries(limit, f, conn.EntryChannel())
}
