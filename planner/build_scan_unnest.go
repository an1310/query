//  Copyright (c) 2016 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package planner

import (
	"github.com/couchbase/query/algebra"
	"github.com/couchbase/query/datastore"
	"github.com/couchbase/query/expression"
	"github.com/couchbase/query/plan"
	"github.com/couchbase/query/value"
)

/*

Algorithm for exploiting array indexes with UNNEST.

Consider only INNER UNNESTs. OUTER UNNESTs cannot exploit array
indexing.

Return a combination of UNNESTs and array indexes that works.

To consider an array index, the array key must be the first key in the
array index, and is the only key exploited for UNNEST.

To find a combination of UNNESTs and array index:

Enumerate all INNER UNNESTs in the FROM clause. Identify the primary
UNNESTs, i.e. those that unnest data in the primary term of the FROM
clause.

Enumerate all array indexes on the primary term having the array key
as their first key. If the index has an index condition, i.e. a WHERE
clause, the query predicate must be a subset of the index
condition. These are the candidate array indexes.

For each primary UNNEST:

1. Find a candidate array index. The array index key must match the
UNNEST; i.e., the array index key is an ALL (DISTINCT) ARRAY
expression whose bindings match the UNNEST's expression and alias.

2. Determine if the index satisfies the current UNNEST, or if the
index should be considered for chained UNNESTs. If the index does not
have further dimensions, i.e. the ARRAY mapping IS NOT another ALL
(DISTINCT) ARRAY expression, then attempt to satisfy the query
predicate using the index. If the index has further dimensions,
i.e. the ARRAY mapping IS another ALL (DISTINCT) ARRAY expression,
then recursively attempt to chain another UNNEST for the index's next
dimension.

*/
func (this *builder) buildUnnestScan(node *algebra.KeyspaceTerm, from algebra.FromTerm,
	pred expression.Expression, indexes map[datastore.Index]*indexEntry) (
	op plan.Operator, sargLength int, err error) {

	// Enumerate INNER UNNESTs
	unnests := _UNNEST_POOL.Get()
	defer _UNNEST_POOL.Put(unnests)
	unnests = collectInnerUnnests(from, unnests)
	if len(unnests) == 0 {
		return nil, 0, nil
	}

	// Enumerate primary UNNESTs
	primaryUnnests := _UNNEST_POOL.Get()
	defer _UNNEST_POOL.Put(primaryUnnests)
	primaryUnnests = collectPrimaryUnnests(from, unnests, primaryUnnests)
	if len(primaryUnnests) == 0 {
		return nil, 0, nil
	}

	// Enumerate candidate array indexes
	unnestIndexes := _INDEX_POOL.Get()
	defer _INDEX_POOL.Put(unnestIndexes)
	unnestIndexes, arrayKeys := collectUnnestIndexes(pred, indexes, unnestIndexes)
	if len(unnestIndexes) == 0 {
		return nil, 0, nil
	}

	// INNER UNNESTs cannot be MISSING
	var andBuf [16]expression.Expression
	var andTerms []expression.Expression
	if 1+len(unnests) <= len(andBuf) {
		andTerms = andBuf[0 : 1+len(unnests)]
	} else {
		andTerms = _AND_POOL.GetSized(1 + len(unnests))
		defer _AND_POOL.Put(andTerms)
	}

	andTerms[0] = pred
	for i, unnest := range unnests {
		andTerms[i+1] = expression.NewIsNotMissing(expression.NewIdentifier(unnest.Alias()))
	}
	pred = expression.NewAnd(andTerms...)

	cops := make(map[datastore.Index]plan.CoveringOperator, len(primaryUnnests))
	cuns := make(map[datastore.Index]map[*algebra.Unnest]bool, len(primaryUnnests))

	for _, index := range unnestIndexes {
		cop, cun, err := this.buildUnnestCoveringScan(node, pred, index, indexes[index], arrayKeys[index], unnests)
		if err != nil {
			return nil, 0, err
		}

		if cop != nil {
			cops[index] = cop
			cuns[index] = cun
		}
	}

	// Find shortest covering scan
	n := 0
	var cop plan.CoveringOperator
	var cun map[*algebra.Unnest]bool
	for index, c := range cops {
		if cop == nil || len(index.RangeKey()) < n {
			cop = c
			cun = cuns[index]
			n = len(index.RangeKey())
			sargLength = len(indexes[index].sargKeys)
		}
	}

	// Return shortest covering scan
	if cop != nil {
		this.coveringScans = append(this.coveringScans, cop)
		this.coveredUnnests = cun

		if len(cun) > 0 {
			return cop, sargLength, nil
		} else {
			return plan.NewDistinctScan(cop), sargLength, nil
		}
	}

	ops := make(map[datastore.Index]*opEntry, len(primaryUnnests))
	for _, unnest := range primaryUnnests {
		for _, index := range unnestIndexes {
			// We already have a covering scan using this index
			if _, ok := cops[index]; ok {
				continue
			}

			arrayKey := arrayKeys[index]
			op, _, n, err = matchUnnest(node, pred, unnest, index, indexes[index], arrayKey, unnests)
			if err != nil {
				return nil, 0, err
			}

			if op == nil {
				continue
			}

			// Keep the longest match for this index
			if entry, ok := ops[index]; ok && entry.Len >= n {
				continue
			} else {
				ops[index] = &opEntry{op, n}
			}
		}
	}

	// No UNNEST scan
	if len(ops) == 0 {
		return nil, 0, nil
	}

	// No pushdowns
	this.resetOrderLimit()
	this.resetCountMin()

	// Eliminate redundant scans
	entries := make(map[datastore.Index]*indexEntry, len(ops))
	for index, _ := range ops {
		entries[index] = indexes[index]
	}

	entries = minimalIndexesUnnest(entries, ops)

	var scanBuf [16]plan.Operator
	var scans []plan.Operator
	if len(entries) <= len(scanBuf) {
		scans = scanBuf[0:0]
	} else {
		scans = make([]plan.Operator, 0, len(entries))
	}

	for index, entry := range entries {
		scans = append(scans, ops[index].Op)
		if len(entry.sargKeys) > sargLength {
			sargLength = len(entry.sargKeys)
		}
	}

	if len(scans) == 1 {
		return scans[0], sargLength, nil
	} else {
		return plan.NewIntersectScan(scans...), sargLength, nil
	}
}

var _AND_POOL = expression.NewExpressionPool(256)

type opEntry struct {
	Op  plan.Operator
	Len int
}

/*
Enumerate INNER UNNEST terms.
*/
func collectInnerUnnests(from algebra.FromTerm, buf []*algebra.Unnest) []*algebra.Unnest {
	joinTerm, ok := from.(algebra.JoinTerm)
	if !ok {
		return buf
	}

	buf = collectInnerUnnests(joinTerm.Left(), buf)

	unnest, ok := joinTerm.(*algebra.Unnest)
	if ok && !unnest.Outer() {
		buf = append(buf, unnest)
	}

	return buf
}

/*
Enumerate primary UNNESTs.
False positives are ok.
*/
func collectPrimaryUnnests(from algebra.FromTerm, unnests, buf []*algebra.Unnest) []*algebra.Unnest {
	primaryAlias := expression.NewIdentifier(from.PrimaryTerm().Alias())
	for _, u := range unnests {
		// This test allows false positives, but that's ok
		if u.Expression().DependsOn(primaryAlias) {
			buf = append(buf, u)
		}
	}

	return buf
}

/*
Enumerate array indexes for UNNEST.
*/
func collectUnnestIndexes(pred expression.Expression, indexes map[datastore.Index]*indexEntry,
	unnestIndexes []datastore.Index) (
	[]datastore.Index, map[datastore.Index]*expression.All) {

	arrayKeys := make(map[datastore.Index]*expression.All, len(indexes))

	for index, entry := range indexes {
		if len(entry.keys) == 0 {
			continue
		}

		firstKey := entry.keys[0]
		all, ok := firstKey.(*expression.All)
		if !ok {
			continue
		}

		if entry.cond != nil &&
			!SubsetOf(pred, entry.cond) {
			continue
		}

		unnestIndexes = append(unnestIndexes, index)
		arrayKeys[index] = all
	}

	return unnestIndexes, arrayKeys
}

func matchUnnest(node *algebra.KeyspaceTerm, pred expression.Expression, unnest *algebra.Unnest,
	index datastore.Index, entry *indexEntry, arrayKey *expression.All, unnests []*algebra.Unnest) (
	plan.Operator, *algebra.Unnest, int, error) {

	array, ok := arrayKey.Array().(*expression.Array)
	if !ok {
		return nil, nil, 0, nil
	}

	if len(array.Bindings()) != 1 {
		return nil, nil, 0, nil
	}

	binding := array.Bindings()[0]
	if unnest.As() != binding.Variable() ||
		!unnest.Expression().EquivalentTo(binding.Expression()) {
		return nil, nil, 0, nil
	}

	arrayMapping := array.ValueMapping()
	nestedArrayKey, ok := arrayMapping.(*expression.All)
	if ok {
		alias := expression.NewIdentifier(unnest.As())
		for _, u := range unnests {
			if u == unnest ||
				!u.Expression().DependsOn(alias) {
				continue
			}

			op, un, n, err := matchUnnest(node, pred, u, index, entry, nestedArrayKey, unnests)
			if op != nil || err != nil {
				return op, un, n + 1, err
			}
		}

		return nil, nil, 0, nil
	} else {
		mappings := expression.Expressions{array.ValueMapping()}
		if SargableFor(pred, mappings) == 0 {
			return nil, nil, 0, nil
		}

		spans, exactSpans, err := SargFor(pred, mappings, len(mappings))
		if err != nil {
			return nil, nil, 0, err
		}

		entry.spans = spans
		entry.exactSpans = exactSpans
		scan := plan.NewIndexScan(index, node, spans, false, nil, nil, nil)
		return plan.NewDistinctScan(scan), unnest, 1, nil
	}
}

func (this *builder) buildUnnestCoveringScan(node *algebra.KeyspaceTerm, pred expression.Expression,
	index datastore.Index, entry *indexEntry, arrayKey *expression.All, unnests []*algebra.Unnest) (
	plan.CoveringOperator, map[*algebra.Unnest]bool, error) {

	// Statement to be covered
	if this.cover == nil {
		return nil, nil, nil
	}

	// Include META().id in covering expressions
	alias := node.Alias()
	id := expression.NewField(
		expression.NewMeta(expression.NewIdentifier(alias)),
		expression.NewFieldName("id", false))

	keys := append(entry.keys, id)

	// Include covering expressions from index WHERE clause
	var filterCovers map[*expression.Cover]value.Value

	if entry.cond != nil {
		var err error
		fc := _FILTER_COVERS_POOL.Get()
		defer _FILTER_COVERS_POOL.Put(fc)
		fc = entry.cond.FilterCovers(fc)
		fc = entry.origCond.FilterCovers(fc)
		filterCovers, err = mapFilterCovers(fc)
		if err != nil {
			return nil, nil, err
		}
	}

	// Allocate covering expressions
	var coveringBuf [64]expression.Expression
	var coveringExprs expression.Expressions
	if len(keys)+len(filterCovers) <= len(coveringBuf) {
		coveringExprs = coveringBuf[0:0]
	} else {
		coveringExprs = make(expression.Expressions, 0, len(keys)+len(filterCovers))
	}

	// Covering expressions from index keys
	for i, key := range keys {
		if i == 0 {
			key = unrollArrayKeys(key)
		}

		coveringExprs = append(coveringExprs, key)
	}

	// Covering expressions from index WHERE clause
	for c, _ := range filterCovers {
		coveringExprs = append(coveringExprs, c.Covered())
	}

	// Array index covers matching UNNEST expressions
	bindings := coveredUnnestBindings(entry.keys[0])
	coveredUnnests := make(map[*algebra.Unnest]bool, len(unnests))
	coveredExprs := make(map[expression.Expression]bool, len(unnests))

	for _, unnest := range unnests {
		unnestExpr := unnest.Expression()
		bindingExpr, ok := bindings[unnest.As()]
		if ok && unnestExpr.EquivalentTo(bindingExpr) {
			coveredUnnests[unnest] = true
			coveredExprs[unnestExpr] = true
		} else {
			coveredUnnests = nil
			coveredExprs = _EMPTY_COVERED_EXPRS
			break
		}
	}

	// Is the statement covered by this index?
	exprs := this.cover.Expressions()
	for _, expr := range exprs {
		_, ok := coveredExprs[expr]
		if !ok && !expr.CoveredBy(alias, coveringExprs) {
			return nil, nil, nil
		}
	}

	covers := make(expression.Covers, 0, len(keys))
	for i, _ := range keys {
		covers = append(covers, expression.NewCover(coveringExprs[i]))
	}

	this.resetOrderLimit()
	this.resetCountMin()

	// Sarg and populate spans
	op, _, _, err := matchUnnest(node, pred, unnests[0], index, entry, arrayKey, unnests)
	if op == nil || err != nil {
		return nil, nil, err
	}

	scan := plan.NewIndexScan(index, node, entry.spans, false, nil, covers, filterCovers)
	return scan, coveredUnnests, nil
}

var _EMPTY_COVERED_EXPRS = make(map[expression.Expression]bool, 0)

func minimalIndexesUnnest(indexes map[datastore.Index]*indexEntry,
	ops map[datastore.Index]*opEntry) map[datastore.Index]*indexEntry {
	for s, se := range indexes {
		for t, te := range indexes {
			if t == s {
				continue
			}

			if narrowerOrEquivalentUnnest(se, te, ops[s], ops[t]) {
				delete(indexes, t)
				delete(ops, t)
			}
		}
	}

	return indexes
}

/*
Is se narrower or equivalent to te.
*/
func narrowerOrEquivalentUnnest(se, te *indexEntry, sop, top *opEntry) bool {
	if top.Len > sop.Len {
		return false
	}

	if te.cond != nil && (se.cond == nil || !SubsetOf(se.cond, te.cond)) {
		return false
	}

outer:
	for _, tk := range te.keys {
		for _, sk := range se.keys {
			if SubsetOf(sk, tk) || sk.DependsOn(tk) {
				continue outer
			}
		}

		return false
	}

	return len(se.keys) <= len(te.keys)
}

func unrollArrayKeys(expr expression.Expression) expression.Expression {
	for all, ok := expr.(*expression.All); ok && !all.Distinct(); all, ok = expr.(*expression.All) {
		if array, ok := all.Array().(*expression.Array); ok &&
			len(array.Bindings()) == 1 && !array.Bindings()[0].Descend() {
			expr = array.ValueMapping()
		} else {
			break
		}
	}

	return expr
}

func coveredUnnestBindings(key expression.Expression) map[string]expression.Expression {
	bindings := make(map[string]expression.Expression, 8)

	for all, ok := key.(*expression.All); ok && !all.Distinct(); all, ok = key.(*expression.All) {
		if array, ok := all.Array().(*expression.Array); ok &&
			len(array.Bindings()) == 1 && !array.Bindings()[0].Descend() {
			binding := array.Bindings()[0]
			bindings[binding.Variable()] = binding.Expression()
			key = array.ValueMapping()
		} else {
			break
		}
	}

	return bindings
}

var _UNNEST_POOL = algebra.NewUnnestPool(8)
