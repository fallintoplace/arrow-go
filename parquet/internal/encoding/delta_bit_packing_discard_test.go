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
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/stretchr/testify/require"
)

func makeDeltaDiscardValues[T int32 | int64](n int, pattern string) []T {
	values := make([]T, n)
	switch pattern {
	case "constant":
		for i := range values {
			values[i] = 27
		}
	case "stride":
		for i := range values {
			values[i] = T(i*3 - 1000)
		}
	case "descending":
		for i := range values {
			values[i] = T(100000 - i*5)
		}
	case "mixed":
		values[0] = 100
		for i := 1; i < n; i++ {
			step := T(3)
			if (i-1)%128 >= 32 && (i-1)%128 < 64 && i%7 == 0 {
				step = 6
			}
			values[i] = values[i-1] + step
		}
	case "wrap":
		values[0] = T(math.MaxInt32 - 16)
		for i := 1; i < n; i++ {
			values[i] = values[i-1] + 7
		}
	case "varying":
		for i := range values {
			values[i] = T(i*17 - i%11)
		}
	default:
		panic("unknown test pattern: " + pattern)
	}
	return values
}

func testDeltaBitPackDiscardZeroWidth[T int32 | int64](t *testing.T, typ parquet.Type) {
	const n = 4097
	for _, pattern := range []string{"constant", "stride", "descending", "mixed", "wrap", "varying"} {
		t.Run(pattern, func(t *testing.T) {
			values := makeDeltaDiscardValues[T](n, pattern)
			encoder := NewEncoder(typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(Encoder[T])
			encoder.Put(values)
			buf, err := encoder.FlushValues()
			require.NoError(t, err)
			defer buf.Release()
			encoder.Release()

			dec := NewDecoder(typ, parquet.Encodings.DeltaBinaryPacked, nil, memory.DefaultAllocator).(*deltaBitPackDecoder[T])
			for _, skip := range []int{0, 1, 31, 32, 33, 34, 63, 64, 65, 66, 127, 128, 129, 130, 1024, 2048, n - 2, n - 1, n} {
				t.Run(fmt.Sprintf("skip_%d", skip), func(t *testing.T) {
					require.NoError(t, dec.SetData(len(values), buf.Bytes()))
					got, err := dec.Discard(skip)
					require.NoError(t, err)
					require.Equal(t, skip, got)
					out := make([]T, len(values)-skip)
					got, err = dec.Decode(out)
					require.NoError(t, err)
					require.Equal(t, len(out), got)
					require.Equal(t, values[skip:], out)
				})
			}

			require.NoError(t, dec.SetData(len(values), buf.Bytes()))
			position := 0
			for _, action := range []struct {
				skip, read int
			}{
				{33, 1}, {32, 2}, {64, 1}, {127, 3}, {n - 263, 0},
			} {
				got, err := dec.Discard(action.skip)
				require.NoError(t, err)
				require.Equal(t, action.skip, got)
				position += action.skip

				out := make([]T, action.read)
				got, err = dec.Decode(out)
				require.NoError(t, err)
				require.Equal(t, action.read, got)
				require.Equal(t, values[position:position+action.read], out)
				position += action.read
			}
		})
	}
}

func TestDeltaBitPackDiscardZeroWidth(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		testDeltaBitPackDiscardZeroWidth[int32](t, parquet.Types.Int32)
	})
	t.Run("int64", func(t *testing.T) {
		testDeltaBitPackDiscardZeroWidth[int64](t, parquet.Types.Int64)
	})
}

func benchmarkDeltaBitPackDiscard[T int32 | int64](b *testing.B, typ parquet.Type, width int) {
	for _, n := range []int{4097, 65537, 1048577} {
		for _, pattern := range []string{"constant", "stride", "mixed", "varying"} {
			b.Run(fmt.Sprintf("%s_%d", pattern, n), func(b *testing.B) {
				values := makeDeltaDiscardValues[T](n, pattern)
				encoder := NewEncoder(typ, parquet.Encodings.DeltaBinaryPacked, false, nil, memory.DefaultAllocator).(Encoder[T])
				encoder.Put(values)
				buf, err := encoder.FlushValues()
				if err != nil {
					b.Fatal(err)
				}
				defer buf.Release()
				encoder.Release()

				dec := NewDecoder(typ, parquet.Encodings.DeltaBinaryPacked, nil, memory.DefaultAllocator).(*deltaBitPackDecoder[T])
				var last [1]T
				b.SetBytes(int64(n * width))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := dec.SetData(n, buf.Bytes()); err != nil {
						b.Fatal(err)
					}
					if got, err := dec.Discard(n - 1); err != nil || got != n-1 {
						b.Fatalf("discarded %d of %d values: %v", got, n-1, err)
					}
					if got, err := dec.Decode(last[:]); err != nil || got != 1 || last[0] != values[n-1] {
						b.Fatalf("wrong value after discard: got %d, value %v, err %v", got, last[0], err)
					}
				}
			})
		}
	}
}

func BenchmarkDeltaBitPackDiscard(b *testing.B) {
	b.Run("int32", func(b *testing.B) {
		benchmarkDeltaBitPackDiscard[int32](b, parquet.Types.Int32, 4)
	})
	b.Run("int64", func(b *testing.B) {
		benchmarkDeltaBitPackDiscard[int64](b, parquet.Types.Int64, 8)
	})
}
