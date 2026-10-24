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
	"bytes"
	"math/rand"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

// Exercise byte-aligned and equally unaligned offsets with disjoint buffers,
// overlapping views, shared storage, and partial first/last bytes.
func TestBitmapSharedAlignmentTransfer(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	lengths := []int{0, 1, 2, 3, 5, 7, 8, 9, 15, 16, 17, 63, 64, 65, 127, 128, 129, 511, 512, 513, 4095}
	layouts := []struct {
		name             string
		srcByte, dstByte int
	}{
		{"separate", 0, 512},
		{"same-range", 0, 0},
		{"forward-overlap", 0, 1},
		{"backward-overlap", 1, 0},
		{"disjoint-same-buffer", 0, 256},
	}

	for _, invert := range []bool{false, true} {
		for srcOffset := 0; srcOffset < 16; srcOffset++ {
			for _, byteShift := range []int{0, 8, 24} {
				dstOffset := srcOffset + byteShift
				for _, length := range lengths {
					for _, layout := range layouts {
						original := make([]byte, 2048)
						_, _ = rng.Read(original)
						got := bytes.Clone(original)
						want := bytes.Clone(original)
						for i := 0; i < length; i++ {
							value := bitutil.BitIsSet(original, layout.srcByte*8+srcOffset+i)
							if invert {
								value = !value
							}
							bitutil.SetBitTo(want, layout.dstByte*8+dstOffset+i, value)
						}

						if invert {
							bitutil.InvertBitmap(got[layout.srcByte:], srcOffset, length, got[layout.dstByte:], dstOffset)
						} else {
							bitutil.CopyBitmap(got[layout.srcByte:], srcOffset, length, got[layout.dstByte:], dstOffset)
						}
						if !bytes.Equal(want, got) {
							t.Fatalf("invert=%t src=%d dst=%d len=%d layout=%s", invert, srcOffset, dstOffset, length, layout.name)
						}
					}
				}
			}
		}
	}
}

// Compare equal bit ranges and mismatches at the final bit. Randomized bytes
// outside the requested range must not affect equality.
func TestBitmapEqualsSharedAlignment(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	lengths := []int64{0, 1, 2, 3, 5, 7, 8, 9, 15, 16, 17, 63, 64, 65, 127, 128, 129, 511, 512, 513, 4095}

	for lOffset := int64(0); lOffset < 16; lOffset++ {
		for _, byteShift := range []int64{0, 8, 24} {
			rOffset := lOffset + byteShift
			for _, length := range lengths {
				for _, mismatch := range []bool{false, true} {
					left := make([]byte, 1024)
					right := make([]byte, 1024)
					_, _ = rng.Read(left)
					_, _ = rng.Read(right)
					for i := int64(0); i < length; i++ {
						bitutil.SetBitTo(right, int(rOffset+i), bitutil.BitIsSet(left, int(lOffset+i)))
					}
					want := true
					if mismatch && length > 0 {
						bitutil.SetBitTo(right, int(rOffset+length-1), !bitutil.BitIsSet(left, int(lOffset+length-1)))
						want = false
					}
					if got := bitutil.BitmapEquals(left, right, lOffset, rOffset, length); got != want {
						t.Fatalf("left=%d right=%d len=%d mismatch=%t: got %t want %t", lOffset, rOffset, length, mismatch, got, want)
					}
				}
			}
		}
	}
}
