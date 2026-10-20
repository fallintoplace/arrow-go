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

package encoding

import (
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/stretchr/testify/require"
)

func testDeltaBitPackZeroWidthRoundTrip[T int32 | int64](t *testing.T, typ parquet.Type) {
	t.Helper()

	maxInt := int64(^uint64(0) >> 1)
	extremes := [...]int64{maxInt, -maxInt - 1, -1, 0, 1}
	for _, n := range []int{1, 2, 31, 32, 33, 127, 128, 129, 256, 1025} {
		for _, pattern := range []string{"constant", "ascending", "descending", "random", "extremes"} {
			t.Run(fmt.Sprintf("%s/n=%d", pattern, n), func(t *testing.T) {
				values := make([]T, n)
				state := uint64(0x9e3779b97f4a7c15)
				for i := range values {
					switch pattern {
					case "constant":
						values[i] = T(42)
					case "ascending":
						values[i] = T(i / 3)
					case "descending":
						values[i] = T(100000 - i*107)
					case "random":
						state ^= state << 13
						state ^= state >> 7
						state ^= state << 17
						values[i] = T(state)
					case "extremes":
						values[i] = T(extremes[i%len(extremes)])
					}
				}

				encoder := NewEncoder(typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(*deltaBitPackEncoder[T])
				// Split values across Put calls and partial/final miniblocks.
				encoder.Put(values[:n/3])
				encoder.Put(values[n/3 : n/2])
				encoder.Put(values[n/2:])
				buf, err := encoder.FlushValues()
				require.NoError(t, err)

				decoder := NewDecoder(typ, parquet.Encodings.DeltaBinaryPacked, nil, memory.DefaultAllocator).(*deltaBitPackDecoder[T])
				require.NoError(t, decoder.SetData(n, buf.Bytes()))
				got := make([]T, n)
				for at := 0; at < n; {
					end := min(at+17, n)
					read, err := decoder.Decode(got[at:end])
					require.NoError(t, err)
					require.Positive(t, read)
					at += read
				}
				require.Equal(t, values, got)

				// Split and unsplit writes must be identical at the byte level.
				whole := NewEncoder(typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(*deltaBitPackEncoder[T])
				whole.Put(values)
				wholeBuf, err := whole.FlushValues()
				require.NoError(t, err)
				require.Equal(t, wholeBuf.Bytes(), buf.Bytes())

				wholeBuf.Release()
				whole.Release()
				buf.Release()
				encoder.Release()
			})
		}
	}
}

func TestDeltaBitPackZeroWidthRoundTrip(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		testDeltaBitPackZeroWidthRoundTrip[int32](t, parquet.Types.Int32)
	})
	t.Run("int64", func(t *testing.T) {
		testDeltaBitPackZeroWidthRoundTrip[int64](t, parquet.Types.Int64)
	})
}

func BenchmarkDeltaBitPackZeroWidthMiniblocks(b *testing.B) {
	for _, test := range []struct {
		name string
		typ  parquet.Type
		size int
	}{
		{"int32", parquet.Types.Int32, 4},
		{"int64", parquet.Types.Int64, 8},
	} {
		for _, pattern := range []string{"ascending", "random"} {
			b.Run(test.name+"/"+pattern, func(b *testing.B) {
				const n = 64 * 1024
				// The encoder's bit packing is independent of the input integer type.
				// Match both widths through their native input slices below.
				b.ReportAllocs()
				b.SetBytes(int64(n * test.size))
				if test.size == 4 {
					values := make([]int32, n)
					for i := range values {
						if pattern == "ascending" {
							values[i] = int32(i)
						} else {
							values[i] = int32(uint32(i)*1664525 + 1013904223)
						}
					}
					enc := NewEncoder(test.typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(*deltaBitPackEncoder[int32])
					defer enc.Release()
					b.ResetTimer()
					for b.Loop() {
						enc.Put(values)
						buf, err := enc.FlushValues()
						if err != nil {
							b.Fatal(err)
						}
						buf.Release()
					}
					return
				}

				values := make([]int64, n)
				for i := range values {
					if pattern == "ascending" {
						values[i] = int64(i)
					} else {
						values[i] = int64(uint64(i)*6364136223846793005 + 1442695040888963407)
					}
				}
				enc := NewEncoder(test.typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(*deltaBitPackEncoder[int64])
				defer enc.Release()
				b.ResetTimer()
				for b.Loop() {
					enc.Put(values)
					buf, err := enc.FlushValues()
					if err != nil {
						b.Fatal(err)
					}
					buf.Release()
				}
			})
		}
	}
}
