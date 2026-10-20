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

//go:build go1.22

package kernels

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func makeRadixTestArray(t *testing.T, dt arrow.DataType, values []int64, pad int, withNulls bool) arrow.Array {
	t.Helper()
	builder := array.NewBuilder(memory.DefaultAllocator, dt)
	defer builder.Release()
	for i, value := range values {
		if withNulls && i >= pad && (i-pad)%97 == 0 {
			builder.AppendNull()
			continue
		}
		switch b := builder.(type) {
		case *array.Int8Builder:
			b.Append(int8(value))
		case *array.Int16Builder:
			b.Append(int16(value))
		case *array.Int32Builder:
			b.Append(int32(value))
		case *array.Int64Builder:
			b.Append(value)
		case *array.Uint8Builder:
			b.Append(uint8(value))
		case *array.Uint16Builder:
			b.Append(uint16(value))
		case *array.Uint32Builder:
			b.Append(uint32(value))
		case *array.Uint64Builder:
			b.Append(uint64(value))
		default:
			t.Fatalf("unsupported test builder %T", builder)
		}
	}
	return builder.NewArray()
}

func radixTestValues(n int) []int64 {
	values := make([]int64, n)
	for i := range values {
		switch i % 11 {
		case 0:
			values[i] = math.MinInt64
		case 1:
			values[i] = math.MaxInt64
		case 2:
			values[i] = -1
		case 3:
			values[i] = 0
		case 4:
			values[i] = 1
		case 5:
			values[i] = int64(i % 7)
		default:
			x := uint64(i+1) * 0x9e3779b97f4a7c15
			values[i] = int64(x ^ (x >> 29))
		}
	}
	return values
}

func TestRadixIntegerSortMatchesStableComparator(t *testing.T) {
	const length = radixSortMinRows + 127
	const pad = 7
	types := []arrow.DataType{
		arrow.PrimitiveTypes.Int8, arrow.PrimitiveTypes.Int16,
		arrow.PrimitiveTypes.Int32, arrow.PrimitiveTypes.Int64,
		arrow.PrimitiveTypes.Uint8, arrow.PrimitiveTypes.Uint16,
		arrow.PrimitiveTypes.Uint32, arrow.PrimitiveTypes.Uint64,
	}
	for _, dt := range types {
		for _, nChunks := range []int{1, 3} {
			for _, order := range []SortOrder{Ascending, Descending} {
				t.Run(fmt.Sprintf("%s/chunks=%d/order=%d", dt, nChunks, order), func(t *testing.T) {
					values := radixTestValues(length)
					padded := make([]int64, length+2*pad)
					copy(padded[pad:], values)
					full := makeRadixTestArray(t, dt, padded, pad, false)
					defer full.Release()
					sliced := array.NewSlice(full, pad, pad+length)
					defer sliced.Release()

					chunks := make([]arrow.Array, 0, nChunks)
					for i := range nChunks {
						start := i * length / nChunks
						end := (i + 1) * length / nChunks
						chunk := array.NewSlice(sliced, int64(start), int64(end))
						defer chunk.Release()
						chunks = append(chunks, chunk)
					}
					if !integerRadixSortable(chunks) {
						t.Fatal("expected physical integer array to use radix sort")
					}

					col := arrow.NewChunked(dt, chunks)
					defer col.Release()
					ctx, _ := newBenchKernelCtx(t)
					key := SortKey{Order: order, NullPlacement: NullsAtEnd}
					expected := make([]uint64, length)
					for i := range expected {
						expected[i] = uint64(i)
					}
					comparator, err := createChunkedComparator(chunks, length, logicalRowMap{})
					if err != nil {
						t.Fatal(err)
					}
					slices.SortStableFunc(expected, func(a, b uint64) int {
						return comparator.compareRowsForKey(a, b, key)
					})

					result, err := SortIndices(ctx, []*arrow.Chunked{col}, []SortKey{key})
					if err != nil {
						t.Fatal(err)
					}
					defer result.Release()
					got := arrow.GetData[uint64](result.Buffers[1].Buf)[:length]
					if !slices.Equal(got, expected) {
						for i := range expected {
							if got[i] != expected[i] {
								t.Fatalf("sort mismatch at index %d: got row %d, expected row %d", i, got[i], expected[i])
							}
						}
					}
				})
			}
		}
	}
}

func TestRadixIntegerSortFallsBackForNullsAndSmallInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		n     int
		nulls bool
	}{
		{"nulls", radixSortMinRows + 25, true},
		{"small", 128, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := radixTestValues(tc.n)
			arr := makeRadixTestArray(t, arrow.PrimitiveTypes.Int64, values, 0, tc.nulls)
			defer arr.Release()
			if tc.nulls && integerRadixSortable([]arrow.Array{arr}) {
				t.Fatal("radix sort must reject arrays with null values")
			}
			col := arrow.NewChunked(arrow.PrimitiveTypes.Int64, []arrow.Array{arr})
			defer col.Release()
			ctx, _ := newBenchKernelCtx(t)
			key := SortKey{Order: Descending, NullPlacement: NullsAtStart}
			result, err := SortIndices(ctx, []*arrow.Chunked{col}, []SortKey{key})
			if err != nil {
				t.Fatal(err)
			}
			defer result.Release()

			expected := make([]uint64, tc.n)
			for i := range expected {
				expected[i] = uint64(i)
			}
			comparator, err := createChunkedComparator([]arrow.Array{arr}, tc.n, logicalRowMap{})
			if err != nil {
				t.Fatal(err)
			}
			slices.SortStableFunc(expected, func(a, b uint64) int {
				return comparator.compareRowsForKey(a, b, key)
			})
			got := arrow.GetData[uint64](result.Buffers[1].Buf)[:tc.n]
			if !slices.Equal(got, expected) {
				t.Fatal("fallback sort result differs from stable comparison sort")
			}
		})
	}
}
