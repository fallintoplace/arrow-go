// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package array_test

import (
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcatSameDictionarySlices(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer mem.AssertSize(t, 0)

	dictType := &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: arrow.BinaryTypes.String}
	backing, err := array.DictArrayFromJSON(mem, dictType, `[0, 1, null, 2, 1, 0]`, `["a", "b", "c"]`)
	require.NoError(t, err)
	defer backing.Release()

	inputs := []arrow.Array{
		array.NewSlice(backing, 1, 5),
		array.NewSlice(backing, 0, 2),
	}
	for _, input := range inputs {
		defer input.Release()
	}

	actual, err := array.Concatenate(inputs, mem)
	require.NoError(t, err)
	defer actual.Release()

	expected, err := array.DictArrayFromJSON(mem, dictType, `[1, null, 2, 1, 0, 1]`, `["a", "b", "c"]`)
	require.NoError(t, err)
	defer expected.Release()

	assert.True(t, array.Equal(expected, actual))
}


func TestConcatSharedHighCardinalityDictionarySlices(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer mem.AssertSize(t, 0)

	const dictionarySize = 1024
	const rows = 4096
	values := make([]string, dictionarySize)
	for i := range values {
		values[i] = fmt.Sprintf("value-%06d", i)
	}
	dictionaryBuilder := array.NewStringBuilder(mem)
	dictionaryBuilder.AppendValues(values, nil)
	dictionary := dictionaryBuilder.NewStringArray()
	dictionaryBuilder.Release()
	defer dictionary.Release()

	indicesValues := make([]int32, rows)
	valid := make([]bool, rows)
	for i := range indicesValues {
		indicesValues[i] = int32(i % dictionarySize)
		valid[i] = i%11 != 0
	}
	indicesBuilder := array.NewInt32Builder(mem)
	indicesBuilder.AppendValues(indicesValues, valid)
	indices := indicesBuilder.NewInt32Array()
	indicesBuilder.Release()
	defer indices.Release()

	dictType := &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: arrow.BinaryTypes.String}
	backing := array.NewDictionaryArray(dictType, indices, dictionary)
	defer backing.Release()

	chunks := []arrow.Array{
		array.NewSlice(backing, 0, 17),
		array.NewSlice(backing, 17, 2000),
		array.NewSlice(backing, 2000, 2000),
		array.NewSlice(backing, 2000, rows),
		array.NewSlice(backing, 0, rows),
	}
	for _, chunk := range chunks {
		defer chunk.Release()
	}

	actual, err := array.Concatenate(chunks, mem)
	require.NoError(t, err)
	defer actual.Release()

	// The zero-length slice must not disrupt the shared dictionary path.
	assert.Equal(t, 2*rows, actual.Len())
	assert.Same(t, backing.Data().Dictionary(), actual.Data().Dictionary())
	// Also verify the combined indices and nulls against a separate array.
	expectedIndicesBuilder := array.NewInt32Builder(mem)
	doubledIndices := append(append([]int32(nil), indicesValues...), indicesValues...)
	doubledValid := append(append([]bool(nil), valid...), valid...)
	expectedIndicesBuilder.AppendValues(doubledIndices, doubledValid)
	expectedIndices := expectedIndicesBuilder.NewInt32Array()
	expectedIndicesBuilder.Release()
	defer expectedIndices.Release()
	expected := array.NewDictionaryArray(dictType, expectedIndices, dictionary)
	defer expected.Release()
	assert.True(t, array.Equal(expected, actual))
}

func TestConcatDistinctDictionaryFallback(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer mem.AssertSize(t, 0)

	dictType := &arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: arrow.BinaryTypes.String}
	first, err := array.DictArrayFromJSON(mem, dictType, `[0, 1, 0]`, `["zero", "one"]`)
	require.NoError(t, err)
	defer first.Release()
	equal, err := array.DictArrayFromJSON(mem, dictType, `[1, 0]`, `["zero", "one"]`)
	require.NoError(t, err)
	defer equal.Release()
	different, err := array.DictArrayFromJSON(mem, dictType, `[0, 1]`, `["one", "two"]`)
	require.NoError(t, err)
	defer different.Release()

	// Distinct but equal dictionaries should retain the original fast path.
	sameResult, err := array.Concatenate([]arrow.Array{first, equal}, mem)
	require.NoError(t, err)
	assert.Same(t, first.Data().Dictionary(), sameResult.Data().Dictionary())
	sameResult.Release()

	// Different dictionaries still require index transposition/unification.
	result, err := array.Concatenate([]arrow.Array{first, equal, different}, mem)
	require.NoError(t, err)
	defer result.Release()
	dictResult := result.(*array.Dictionary)
	dictValues := dictResult.Dictionary().(*array.String)
	want := []string{"zero", "one", "zero", "one", "zero", "one", "two"}
	for i, value := range want {
		assert.Equal(t, value, dictValues.Value(dictResult.GetValueIndex(i)), "row %d", i)
	}
}
