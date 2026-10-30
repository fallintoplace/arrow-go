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

func TestCountSetBitsShortRanges(t *testing.T) {
	for storageOffset := 0; storageOffset < 8; storageOffset++ {
		for offset := 0; offset <= 15; offset++ {
			for _, n := range []int{0, 1, 2, 3, 7, 8, 9, 15, 16, 31, 32, 33, 63, 64, 65, 66, 127, 128, 129} {
				storage := make([]byte, storageOffset+(offset+n+7)/8)
				bitmap := storage[storageOffset:]
				for i := range bitmap {
					bitmap[i] = byte(i*73 + 31)
				}

				want := 0
				for i := offset; i < offset+n; i++ {
					if bitutil.BitIsSet(bitmap, i) {
						want++
					}
				}
				if got := bitutil.CountSetBits(bitmap, offset, n); got != want {
					t.Fatalf("storageOffset=%d offset=%d n=%d: got=%d want=%d",
						storageOffset, offset, n, got, want)
				}
			}
		}
	}
}

var countSetBitsShortResult int

func BenchmarkCountSetBitsShortRanges(b *testing.B) {
	for _, n := range []int{7, 31, 63, 64, 65, 127, 1023, 65535} {
		for _, offset := range []int{0, 3} {
			b.Run(fmt.Sprintf("bits=%d/offset=%d", n, offset), func(b *testing.B) {
				bitmap := make([]byte, (offset+n+7)/8)
				for i := range bitmap {
					bitmap[i] = byte(i*73 + 31)
				}

				b.ReportAllocs()
				var result int
				for b.Loop() {
					result = bitutil.CountSetBits(bitmap, offset, n)
				}
				countSetBitsShortResult = result
			})
		}
	}
}
