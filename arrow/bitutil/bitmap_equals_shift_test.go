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

package bitutil

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
)

func referenceBitmapEquals(left, right []byte, lOffset, rOffset, length int64) bool {
	for i := int64(0); i < length; i++ {
		if BitIsSet(left, int(lOffset+i)) != BitIsSet(right, int(rOffset+i)) {
			return false
		}
	}
	return true
}

func TestBitmapEqualsShiftedOffsets(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	for _, length := range []int{0, 1, 2, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129, 256, 4096} {
		for lOffset := 0; lOffset < 24; lOffset++ {
			for rOffset := 0; rOffset < 24; rOffset++ {
				left := make([]byte, (lOffset+length+7)/8+1)
				right := make([]byte, (rOffset+length+7)/8+1)
				rng.Read(left)
				rng.Read(right)
				for i := 0; i < length; i++ {
					j := rOffset + i
					mask := byte(1 << uint(j%8))
					if BitIsSet(left, lOffset+i) {
						right[j/8] |= mask
					} else {
						right[j/8] &^= mask
					}
				}

				if !BitmapEquals(left, right, int64(lOffset), int64(rOffset), int64(length)) {
					t.Fatalf("equal: len=%d left=%d right=%d", length, lOffset, rOffset)
				}
				if !referenceBitmapEquals(left, right, int64(lOffset), int64(rOffset), int64(length)) {
					t.Fatalf("invalid fixture: len=%d left=%d right=%d", length, lOffset, rOffset)
				}
				if length == 0 {
					continue
				}
				for _, idx := range []int{0, length / 2, length - 1} {
					j := rOffset + idx
					right[j/8] ^= 1 << uint(j%8)
					if BitmapEquals(left, right, int64(lOffset), int64(rOffset), int64(length)) {
						t.Fatalf("mismatch: len=%d left=%d right=%d changed=%d", length, lOffset, rOffset, idx)
					}
					right[j/8] ^= 1 << uint(j%8)
				}
			}
		}
	}
	if !BitmapEquals(nil, nil, 0, 0, 0) {
		t.Fatal("empty bitmaps should compare equal")
	}
}

// Keep the previous production algorithm in benchmarks for reproducible
// before/after comparisons on the same toolchain and CPU.
func bitmapEqualsBefore(left, right []byte, lOffset, rOffset, length int64) bool {
	if lOffset%8 == 0 && rOffset%8 == 0 {
		byteLen := length / 8
		lStart := lOffset / 8
		rStart := rOffset / 8
		if !bytes.Equal(left[lStart:lStart+byteLen], right[rStart:rStart+byteLen]) {
			return false
		}
		for i := (length / 8) * 8; i < length; i++ {
			if BitIsSet(left, int(lOffset+i)) != BitIsSet(right, int(rOffset+i)) {
				return false
			}
		}
		return true
	}

	lrdr := NewBitmapWordReader(left, int(lOffset), int(length))
	rrdr := NewBitmapWordReader(right, int(rOffset), int(length))
	nwords := lrdr.Words()
	for nwords > 0 {
		nwords--
		if lrdr.NextWord() != rrdr.NextWord() {
			return false
		}
	}
	nbytes := lrdr.TrailingBytes()
	for nbytes > 0 {
		nbytes--
		lbt, _ := lrdr.NextTrailingByte()
		rbt, _ := rrdr.NextTrailingByte()
		if lbt != rbt {
			return false
		}
	}
	return true
}

var bitmapEqualsBenchResult bool

func BenchmarkBitmapEqualsOffsets(b *testing.B) {
	for _, length := range []int{16, 256, 4096, 65536, 1048576} {
		for _, offsets := range [][2]int{{0, 0}, {3, 3}, {1, 5}, {7, 0}} {
			lOffset, rOffset := offsets[0], offsets[1]
			left := make([]byte, (lOffset+length+7)/8)
			right := make([]byte, (rOffset+length+7)/8)
			rng := rand.New(rand.NewSource(45678))
			rng.Read(left)
			for i := 0; i < length; i++ {
				if BitIsSet(left, lOffset+i) {
					j := rOffset + i
					right[j/8] |= 1 << uint(j%8)
				}
			}
			for _, mismatch := range []bool{false, true} {
				if mismatch {
					j := rOffset + length - 1
					right[j/8] ^= 1 << uint(j%8)
				}
				kind := "equal"
				if mismatch {
					kind = "last-mismatch"
				}
				for _, impl := range []struct {
					name string
					run  func([]byte, []byte, int64, int64, int64) bool
				}{{"before", bitmapEqualsBefore}, {"after", BitmapEquals}} {
					b.Run(fmt.Sprintf("bits=%d/offset=%d-%d/%s/%s", length, lOffset, rOffset, kind, impl.name), func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							bitmapEqualsBenchResult = impl.run(left, right, int64(lOffset), int64(rOffset), int64(length))
						}
					})
				}
				if mismatch {
					j := rOffset + length - 1
					right[j/8] ^= 1 << uint(j%8)
				}
			}
		}
	}
}
