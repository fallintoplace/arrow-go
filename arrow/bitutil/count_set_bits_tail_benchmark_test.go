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

// Check all possible partial-byte patterns, including unrelated bits before
// and after the requested range and tails following complete 64-bit words.
func TestCountSetBitsTrailingByteMask(t *testing.T) {
	for _, wholeBytes := range []int{0, 1, 7, 8, 16} {
		for offset := 0; offset < 8; offset++ {
			for tail := 1; tail < 8; tail++ {
				length := wholeBytes*8 + tail
				buf := make([]byte, (offset+length+7)/8)
				for i := range buf {
					buf[i] = byte(i*37 + 13)
				}
				for pattern := 0; pattern < 256; pattern++ {
					buf[len(buf)-1] = byte(pattern)
					want := 0
					for i := 0; i < length; i++ {
						if bitutil.BitIsSet(buf, offset+i) {
							want++
						}
					}
					if got := bitutil.CountSetBits(buf, offset, length); got != want {
						t.Fatalf("wholeBytes=%d offset=%d tail=%d pattern=%d: got=%d want=%d", wholeBytes, offset, tail, pattern, got, want)
					}
				}
			}
		}
	}
}

var countSetBitsTailBenchmarkSink int

func BenchmarkCountSetBitsTrailingByte(b *testing.B) {
	for _, offset := range []int{0, 3} {
		for _, length := range []int{1, 3, 7, 8, 15, 31, 63, 64, 65, 127, 255, 1023, 8191, 65535} {
			b.Run(fmt.Sprintf("offset=%d/length=%d", offset, length), func(b *testing.B) {
				bitmap := make([]byte, (offset+length+7)/8)
				for i := range bitmap {
					bitmap[i] = byte(i*71 + 121)
				}
				b.SetBytes(int64((length + 7) / 8))
				b.ReportAllocs()
				var result int
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					result = bitutil.CountSetBits(bitmap, offset, length)
				}
				countSetBitsTailBenchmarkSink = result
			})
		}
	}
}
