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
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

func BenchmarkSortIndicesInt64RadixPatterns(b *testing.B) {
	const rows = 65536
	for _, pattern := range []string{"random", "duplicates", "ascending", "descending"} {
		b.Run(pattern, func(b *testing.B) {
			ctx, mem := newBenchKernelCtx(b)
			builder := array.NewInt64Builder(mem)
			for i := 0; i < rows; i++ {
				var value int64
				switch pattern {
				case "random":
					value = int64(uint64(i) * 0x9e3779b97f4a7c15)
				case "duplicates":
					value = int64(i%32 - 16)
				case "ascending":
					value = int64(i)
				case "descending":
					value = int64(rows - i)
				}
				builder.Append(value)
			}
			arr := builder.NewArray()
			builder.Release()
			col := arrow.NewChunked(arrow.PrimitiveTypes.Int64, []arrow.Array{arr})
			arr.Release()
			defer col.Release()

			b.ReportAllocs()
			b.SetBytes(rows * arrow.Int64SizeBytes)
			b.ResetTimer()
			for range b.N {
				result, err := SortIndices(ctx, []*arrow.Chunked{col},
					[]SortKey{{Order: Ascending, NullPlacement: NullsAtEnd}})
				if err != nil {
					b.Fatal(err)
				}
				result.Release()
			}
		})
	}
}
