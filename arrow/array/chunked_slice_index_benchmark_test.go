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

	"github.com/apache/arrow-go/v18/arrow/array"
)

func BenchmarkNewChunkedSliceShortRange(b *testing.B) {
	const rowsPerChunk = 64
	for _, numChunks := range []int{64, 1024, 8192} {
		for _, position := range []string{"head", "middle", "tail"} {
			for _, rows := range []int64{1, 64} {
				b.Run(fmt.Sprintf("chunks=%d/%s/rows=%d", numChunks, position, rows), func(b *testing.B) {
					input := makeChunkedSliceBenchmarkInput(numChunks, rowsPerChunk, false)
					defer input.Release()

					var start, end int64
					switch position {
					case "head":
						start, end = 0, rows
					case "middle":
						start = int64(input.Len())/2
						end = start + rows
					default:
						end = int64(input.Len())
						start = end - rows
					}

					// Exclude one-time lazy index construction from steady-state results.
					warm := array.NewChunkedSlice(input, start, end)
					warm.Release()
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						result := array.NewChunkedSlice(input, start, end)
						benchmarkNewChunkedSliceSink = result.Len()
						result.Release()
					}
				})
			}
		}
	}
}
