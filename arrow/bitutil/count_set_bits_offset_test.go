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
	"math/rand"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

func TestCountSetBitsUnalignedRanges(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	lengths := []int{0, 1, 2, 3, 7, 8, 9, 15, 16, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 1023, 1024, 4095, 4096}

	for _, length := range lengths {
		for offset := 0; offset < 130; offset++ {
			// Allocate exactly the number of bytes required for the range.
			// A word-based implementation must not read past this slice.
			bitmap := make([]byte, (offset+length+7)/8)
			for pattern := 0; pattern < 3; pattern++ {
				switch pattern {
				case 0:
					clear(bitmap)
				case 1:
					for i := range bitmap {
						bitmap[i] = 0xff
					}
				default:
					if _, err := rng.Read(bitmap); err != nil {
						t.Fatal(err)
					}
				}

				want := 0
				for i := offset; i < offset+length; i++ {
					if bitutil.BitIsSet(bitmap, i) {
						want++
					}
				}
				if got := bitutil.CountSetBits(bitmap, offset, length); got != want {
					t.Fatalf("offset=%d length=%d pattern=%d: got %d, want %d", offset, length, pattern, got, want)
				}
			}
		}
	}
}

func TestCountSetBitsUnalignedLargeRanges(t *testing.T) {
	for _, offset := range []int{1, 7, 13, 63, 64, 65, 127} {
		const length = 65536
		bitmap := make([]byte, (offset+length+7)/8)
		for i := range bitmap {
			bitmap[i] = byte(i*37 + 13)
		}

		want := 0
		for i := offset; i < offset+length; i++ {
			if bitutil.BitIsSet(bitmap, i) {
				want++
			}
		}
		if got := bitutil.CountSetBits(bitmap, offset, length); got != want {
			t.Fatalf("offset=%d length=%d: got %d, want %d", offset, length, got, want)
		}
	}
}

var countSetBitsUnalignedSink int

func BenchmarkCountSetBitsUnaligned(b *testing.B) {
	for _, tc := range []struct {
		offset, length int
	}{
		{0, 128}, {1, 128}, {7, 128}, {13, 128}, {63, 128},
		{1, 1024}, {7, 1024}, {13, 1024}, {1, 4096},
		{1, 65536}, {13, 65536}, {63, 65536},
	} {
		b.Run(fmt.Sprintf("offset=%d/length=%d", tc.offset, tc.length), func(b *testing.B) {
			bitmap := make([]byte, (tc.offset+tc.length+7)/8)
			for i := range bitmap {
				bitmap[i] = byte(i*37 + 13)
			}

			b.SetBytes(int64(tc.length / 8))
			b.ReportAllocs()
			b.ResetTimer()
			var count int
			for i := 0; i < b.N; i++ {
				count = bitutil.CountSetBits(bitmap, tc.offset, tc.length)
			}
			countSetBitsUnalignedSink = count
		})
	}
}
