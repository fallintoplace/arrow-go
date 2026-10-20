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

package bitutil_test

import (
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

func TestCountSetBitsOffsetBoundaries(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 7, 8, 9, 15, 16, 31, 32, 63, 64, 65, 127, 128, 129, 511, 512, 513}
	for _, size := range []int{0, 1, 2, 7, 8, 9, 16, 63, 64, 65, 129} {
		for _, pattern := range []string{"zeros", "ones", "mixed"} {
			bitmap := make([]byte, size)
			for i := range bitmap {
				switch pattern {
				case "ones":
					bitmap[i] = 0xff
				case "mixed":
					bitmap[i] = byte((i*37 + size*13) ^ (i >> 2))
				}
			}

			for offset := 0; offset <= size*8; offset++ {
				for _, n := range append(lengths, size*8-offset) {
					if offset+n > size*8 {
						continue
					}
					want := 0
					for i := offset; i < offset+n; i++ {
						if bitutil.BitIsSet(bitmap, i) {
							want++
						}
					}
					if got := bitutil.CountSetBits(bitmap, offset, n); got != want {
						t.Fatalf("size=%d pattern=%s offset=%d n=%d: got %d, want %d",
							size, pattern, offset, n, got, want)
					}
				}
			}
		}
	}

	if got := bitutil.CountSetBits(nil, 100, 0); got != 0 {
		t.Fatalf("empty range: got %d, want 0", got)
	}
}

var countSetBitsOffsetBenchmarkResult int

func BenchmarkCountSetBitsOffsetCases(b *testing.B) {
	for _, n := range []int{16, 64, 1024, 65536} {
		for _, offset := range []int{0, 1, 7, 31, 63} {
			b.Run(fmt.Sprintf("bits=%d/offset=%d", n, offset), func(b *testing.B) {
				bitmap := make([]byte, int(bitutil.BytesForBits(int64(offset+n))))
				for i := range bitmap {
					bitmap[i] = byte(i*37 + 91)
				}
				b.SetBytes(int64(bitutil.BytesForBits(int64(n))))
				b.ReportAllocs()
				result := 0
				for i := 0; i < b.N; i++ {
					result = bitutil.CountSetBits(bitmap, offset, n)
				}
				countSetBitsOffsetBenchmarkResult = result
			})
		}
	}
}
