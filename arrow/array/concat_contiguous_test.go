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
)

func TestConcatenateAdjacentSlicesCopyBuffers(t *testing.T) {
	const total = 513
	tests := []struct {
		name  string
		build func() arrow.Array
	}{
		{
			name: "int64-with-nulls",
			build: func() arrow.Array {
				b := array.NewInt64Builder(memory.DefaultAllocator)
				defer b.Release()
				values := make([]int64, total)
				valid := make([]bool, total)
				for i := range values {
					values[i] = int64(i * 19)
					valid[i] = i%11 != 0
				}
				b.AppendValues(values, valid)
				return b.NewInt64Array()
			},
		},
		{
			name: "boolean-with-nulls",
			build: func() arrow.Array {
				b := array.NewBooleanBuilder(memory.DefaultAllocator)
				defer b.Release()
				values := make([]bool, total)
				valid := make([]bool, total)
				for i := range values {
					values[i] = i%3 == 0
					valid[i] = i%11 != 0
				}
				b.AppendValues(values, valid)
				return b.NewBooleanArray()
			},
		},
		{
			name: "variable-width-with-nulls",
			build: func() arrow.Array {
				b := array.NewStringBuilder(memory.DefaultAllocator)
				defer b.Release()
				values := make([]string, total)
				valid := make([]bool, total)
				for i := range values {
					values[i] = fmt.Sprintf("value-%d", i)
					if i%5 == 0 {
						values[i] = ""
					}
					valid[i] = i%11 != 0
				}
				b.AppendValues(values, valid)
				return b.NewStringArray()
			},
		},
		{
			name: "variable-width-empty-values",
			build: func() arrow.Array {
				b := array.NewStringBuilder(memory.DefaultAllocator)
				defer b.Release()
				values := make([]string, total)
				b.AppendValues(values, nil)
				return b.NewStringArray()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backing := tt.build()
			defer backing.Release()

			const first, count, step = 5, 64, 7
			expected := array.NewSlice(backing, first, first+count*step)
			defer expected.Release()

			slices := make([]arrow.Array, 0, count+1)
			for i := 0; i < count; i++ {
				start := int64(first + i*step)
				slices = append(slices, array.NewSlice(backing, start, start+step))
				if i == count/2 {
					slices = append(slices, array.NewSlice(backing, start+step, start+step))
				}
			}
			defer func() {
				for _, a := range slices {
					a.Release()
				}
			}()

			got, err := array.Concatenate(slices, memory.DefaultAllocator)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Release()

			if !array.Equal(got, expected) {
				t.Fatalf("adjacent slices returned wrong values (%s)", tt.name)
			}
			for i, input := range backing.Data().Buffers() {
				if input != nil && i < len(got.Data().Buffers()) && input == got.Data().Buffers()[i] {
					t.Fatalf("output unexpectedly shares buffer %d with input", i)
				}
			}
		})
	}
}

func TestConcatenateDisjointSlicesFallback(t *testing.T) {
	const total, chunks, step, gap = 1024, 32, 8, 8
	builder := array.NewInt64Builder(memory.DefaultAllocator)
	defer builder.Release()
	values := make([]int64, total)
	valid := make([]bool, total)
	for i := range values {
		values[i] = int64(i)
		valid[i] = i%9 != 0
	}
	builder.AppendValues(values, valid)
	backing := builder.NewInt64Array()
	defer backing.Release()

	wantValues := make([]int64, 0, chunks*step)
	wantValid := make([]bool, 0, chunks*step)
	inputs := make([]arrow.Array, 0, chunks)
	for i := 0; i < chunks; i++ {
		start := i * (step + gap)
		inputs = append(inputs, array.NewSlice(backing, int64(start), int64(start+step)))
		wantValues = append(wantValues, values[start:start+step]...)
		wantValid = append(wantValid, valid[start:start+step]...)
	}
	defer func() {
		for _, a := range inputs {
			a.Release()
		}
	}()

	expectedBuilder := array.NewInt64Builder(memory.DefaultAllocator)
	defer expectedBuilder.Release()
	expectedBuilder.AppendValues(wantValues, wantValid)
	expected := expectedBuilder.NewInt64Array()
	defer expected.Release()

	got, err := array.Concatenate(inputs, memory.DefaultAllocator)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	if !array.Equal(got, expected) {
		t.Fatal("disjoint slices were incorrectly bulk copied")
	}
}

func TestConcatenateMixedBitmapFallback(t *testing.T) {
	const total, chunks, step = 256, 32, 8
	makeArray := func(withNulls bool) arrow.Array {
		b := array.NewBooleanBuilder(memory.DefaultAllocator)
		defer b.Release()
		values := make([]bool, total)
		valid := make([]bool, total)
		for i := range values {
			values[i] = i%3 == 0
			valid[i] = !withNulls || i%7 != 0
		}
		b.AppendValues(values, valid)
		return b.NewBooleanArray()
	}

	a := makeArray(true)
	defer a.Release()
	b := makeArray(false)
	defer b.Release()

	inputs := make([]arrow.Array, 0, chunks)
	wantValues := make([]bool, total)
	wantValid := make([]bool, total)
	for i := 0; i < chunks; i++ {
		src := a
		if i%2 != 0 {
			src = b
		}
		start := i * step
		inputs = append(inputs, array.NewSlice(src, int64(start), int64(start+step)))
		for j := start; j < start+step; j++ {
			wantValues[j] = j%3 == 0
			wantValid[j] = i%2 != 0 || j%7 != 0
		}
	}
	defer func() {
		for _, input := range inputs {
			input.Release()
		}
	}()

	expectedBuilder := array.NewBooleanBuilder(memory.DefaultAllocator)
	defer expectedBuilder.Release()
	expectedBuilder.AppendValues(wantValues, wantValid)
	expected := expectedBuilder.NewBooleanArray()
	defer expected.Release()

	got, err := array.Concatenate(inputs, memory.DefaultAllocator)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	if !array.Equal(got, expected) {
		t.Fatal("mixed bitmap-backed slices returned wrong values")
	}
}

func BenchmarkConcatenateDisjointSlices(b *testing.B) {
	const total = 65536
	builder := array.NewInt64Builder(memory.DefaultAllocator)
	builder.AppendValues(make([]int64, total), nil)
	backing := builder.NewInt64Array()
	builder.Release()
	defer backing.Release()

	for _, count := range []int{64, 1024, 8192} {
		b.Run(fmt.Sprintf("chunks-%d", count), func(b *testing.B) {
			step := total / count / 2
			inputs := make([]arrow.Array, count)
			for i := range inputs {
				start := int64(i * 2 * step)
				inputs[i] = array.NewSlice(backing, start, start+int64(step))
			}
			defer func() {
				for _, input := range inputs {
					input.Release()
				}
			}()
			b.ReportAllocs()
			b.SetBytes(int64(count * step * 8))
			b.ResetTimer()
			for b.Loop() {
				result, err := array.Concatenate(inputs, memory.DefaultAllocator)
				if err != nil {
					b.Fatal(err)
				}
				result.Release()
			}
		})
	}
}
