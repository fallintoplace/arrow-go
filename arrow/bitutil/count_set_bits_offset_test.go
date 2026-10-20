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
	"math/rand"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

func TestCountSetBitsUnalignedBoundaries(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for size := 0; size <= 72; size++ {
		for _, pattern := range []string{"zero", "ones", "random"} {
			buf := make([]byte, size)
			switch pattern {
			case "ones":
				for i := range buf {
					buf[i] = 0xff
				}
			case "random":
				if _, err := rng.Read(buf); err != nil {
					t.Fatal(err)
				}
			}

			for offset := 0; offset <= len(buf)*8; offset++ {
				available := len(buf)*8 - offset
				for _, n := range []int{0, 1, 2, 3, 7, 8, 9, 31, 32, 33, 63, 64, 65, available} {
					if n > available {
						continue
					}
					want := 0
					for i := offset; i < offset+n; i++ {
						if buf[i/8]&(1<<uint(i&7)) != 0 {
							want++
						}
					}
					if got := bitutil.CountSetBits(buf, offset, n); got != want {
						t.Fatalf("pattern=%s size=%d offset=%d n=%d got=%d want=%d",
							pattern, size, offset, n, got, want)
					}
				}
			}
		}
	}
}

var countSetBitsBenchmarkSink int

func BenchmarkCountSetBitsUnaligned(b *testing.B) {
	buf := make([]byte, 4096+16)
	for i := range buf {
		buf[i] = byte(i*37 + 11)
	}

	for _, tc := range []struct {
		name   string
		offset int
		n      int
	}{
		{"7bits_offset1", 1, 7},
		{"63bits_offset1", 1, 63},
		{"96bits_offset31", 31, 96},
		{"256bits_offset63", 63, 256},
		{"256bits_aligned", 64, 256},
		{"32768bits_offset3", 3, 4096 * 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				countSetBitsBenchmarkSink = bitutil.CountSetBits(buf, tc.offset, tc.n)
			}
		})
	}
}
