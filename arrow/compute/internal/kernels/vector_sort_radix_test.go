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
	"cmp"
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute/exec"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func compareRadixTestValues(arr arrow.Array, i, j int) int {
	switch a := arr.(type) {
	case *array.Int8:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Int16:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Int32:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Int64:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Uint8:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Uint16:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Uint32:
		return cmp.Compare(a.Value(i), a.Value(j))
	case *array.Uint64:
		return cmp.Compare(a.Value(i), a.Value(j))
	default:
		panic("unsupported radix test array")
	}
}

func buildRadixTestArray(t *testing.T, mem memory.Allocator, dt arrow.DataType, rows int, pattern string) arrow.Array {
	t.Helper()
	b := array.NewBuilder(mem, dt)
	defer b.Release()
	for i := 0; i < rows+3; i++ {
		var raw int64
		switch pattern {
		case "ascending":
			raw = int64(i)
		case "descending":
			raw = int64(rows + 3 - i)
		case "equal":
			raw = 7
		case "duplicates":
			raw = int64((i*19)%15 - 7)
		default:
			raw = int64(uint64(i)*0x9e3779b97f4a7c15)
			if i%13 == 0 {
				raw = 0
			}
		}

		var s string
		switch dt.ID() {
		case arrow.INT8:
			s = strconv.FormatInt(int64(int8(raw)), 10)
		case arrow.INT16:
			s = strconv.FormatInt(int64(int16(raw)), 10)
		case arrow.INT32:
			s = strconv.FormatInt(int64(int32(raw)), 10)
		case arrow.INT64:
			s = strconv.FormatInt(raw, 10)
		case arrow.UINT8:
			s = strconv.FormatUint(uint64(uint8(raw)), 10)
		case arrow.UINT16:
			s = strconv.FormatUint(uint64(uint16(raw)), 10)
		case arrow.UINT32:
			s = strconv.FormatUint(uint64(uint32(raw)), 10)
		case arrow.UINT64:
			s = strconv.FormatUint(uint64(raw), 10)
		}
		if err := b.AppendValueFromString(s); err != nil {
			t.Fatalf("append %s: %v", dt, err)
		}
	}
	full := b.NewArray()
	sliced := array.NewSlice(full, 3, int64(rows+3))
	full.Release()
	return sliced
}

func TestSortIndicesIntegerRadix(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer mem.AssertSize(t, 0)
	ctx := &exec.KernelCtx{Ctx: exec.WithAllocator(context.Background(), mem)}

	for _, dt := range []arrow.DataType{
		arrow.PrimitiveTypes.Int8, arrow.PrimitiveTypes.Int16,
		arrow.PrimitiveTypes.Int32, arrow.PrimitiveTypes.Int64,
		arrow.PrimitiveTypes.Uint8, arrow.PrimitiveTypes.Uint16,
		arrow.PrimitiveTypes.Uint32, arrow.PrimitiveTypes.Uint64,
	} {
		for _, pattern := range []string{"random", "duplicates", "ascending", "descending", "equal"} {
			for _, rows := range []int{33, 4096} {
				t.Run(dt.String()+"/"+pattern+"/"+strconv.Itoa(rows), func(t *testing.T) {
					arr := buildRadixTestArray(t, mem, dt, rows, pattern)
					defer arr.Release()
					chunked := arrow.NewChunked(dt, []arrow.Array{arr})
					defer chunked.Release()

					for _, order := range []SortOrder{Ascending, Descending} {
						result, err := SortIndices(ctx, []*arrow.Chunked{chunked},
							[]SortKey{{ColumnIndex: 0, Order: order, NullPlacement: NullsAtEnd}})
						if err != nil {
							t.Fatal(err)
						}
						got := slices.Clone(arrow.GetData[uint64](result.Buffers[1].Buf)[:rows])
						result.Release()

						want := make([]uint64, rows)
						for i := range want {
							want[i] = uint64(i)
						}
						slices.SortStableFunc(want, func(i, j uint64) int {
							c := compareRadixTestValues(arr, int(i), int(j))
							if order == Descending {
								return -c
							}
							return c
						})
						if !slices.Equal(got, want) {
							t.Fatalf("order=%d values=%s: got incorrect indices", order, dt)
						}
					}
				})
			}
		}
	}
}
