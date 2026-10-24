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

func BenchmarkBitmapSharedAlignmentTransfer(b *testing.B) {
	operations := []struct {
		name string
		fn   func([]byte, int, int, []byte, int)
	}{
		{"copy", bitutil.CopyBitmap},
		{"invert", bitutil.InvertBitmap},
	}
	offsets := []struct {
		name     string
		src, dst int
	}{
		{"aligned", 0, 0},
		{"matching", 3, 3},
		{"matching-different-bytes", 3, 11},
		{"different-bits-control", 3, 5},
	}
	for _, operation := range operations {
		for _, nbytes := range []int{64, 4096, 65536} {
			for _, offset := range offsets {
				b.Run(fmt.Sprintf("%s/bytes=%d/%s", operation.name, nbytes, offset.name), func(b *testing.B) {
					src := make([]byte, nbytes+32)
					dst := make([]byte, nbytes+32)
					for i := range src {
						src[i] = byte(i*17 + 23)
					}
					length := nbytes*8 - 32
					b.SetBytes(int64(length / 8))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						operation.fn(src, offset.src, length, dst, offset.dst)
					}
				})
			}
		}
	}
}

var sharedAlignmentEqualsSink bool

func BenchmarkBitmapSharedAlignmentEquals(b *testing.B) {
	offsets := []struct {
		name        string
		left, right int
	}{
		{"aligned", 0, 0},
		{"matching", 3, 3},
		{"matching-different-bytes", 3, 11},
		{"different-bits-control", 3, 5},
	}
	for _, nbytes := range []int{64, 4096, 65536} {
		for _, offset := range offsets {
			for _, mismatch := range []bool{false, true} {
				b.Run(fmt.Sprintf("bytes=%d/%s/mismatch=%t", nbytes, offset.name, mismatch), func(b *testing.B) {
					left := make([]byte, nbytes+32)
					right := make([]byte, nbytes+32)
					for i := range left {
						left[i] = byte(i*17 + 23)
					}
					length := nbytes*8 - 32
					bitutil.CopyBitmap(left, offset.left, length, right, offset.right)
					if mismatch {
						lastBit := offset.right + length - 1
						bitutil.SetBitTo(right, lastBit, !bitutil.BitIsSet(right, lastBit))
					}
					b.SetBytes(int64(length / 8))
					b.ReportAllocs()
					b.ResetTimer()
					var equal bool
					for i := 0; i < b.N; i++ {
						equal = bitutil.BitmapEquals(left, right, int64(offset.left), int64(offset.right), int64(length))
					}
					sharedAlignmentEqualsSink = equal
				})
			}
		}
	}
}
