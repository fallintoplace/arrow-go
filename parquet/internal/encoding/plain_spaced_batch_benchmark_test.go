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

package encoding

import (
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

func makePlainSpacedBenchBitmap(n int, pattern string) []byte {
	if pattern == "nil" {
		return nil
	}
	const offset = 3
	valid := make([]byte, bitutil.BytesForBits(int64(n+offset)))
	for i := 0; i < n; i++ {
		ok := false
		switch pattern {
		case "alternating":
			ok = i%2 == 0
		case "sparse":
			ok = i%8 == 1
		case "runs":
			ok = i%128 < 64
		}
		if ok {
			bitutil.SetBit(valid, offset+i)
		}
	}
	return valid
}

// BenchmarkPlainSpacedBatch measures only encoding. The destination buffer is
// pre-reserved and reused, so this isolates valid-run processing from allocator
// and flush costs. Compare the same file on base and head with -count=3.
func BenchmarkPlainSpacedBatch(b *testing.B) {
	const n = 65536
	const bitOffset = int64(3)

	for _, path := range []string{"arrow32", "arrow64", "bytearray", "fixed-arrow", "fixed-values"} {
		for _, width := range []int{8, 64} {
			for _, pattern := range []string{"nil", "alternating", "sparse", "runs"} {
				b.Run(fmt.Sprintf("%s/width=%d/%s", path, width, pattern), func(b *testing.B) {
					valid := makePlainSpacedBenchBitmap(n, pattern)
					var encode func()
					var reset func()
					var release func()
					if path == "fixed-arrow" || path == "fixed-values" {
						col := schema.NewColumn(schema.NewFixedLenByteArrayNode("value", parquet.Repetitions.Required, int32(width), -1), 0, 0)
						enc := NewEncoder(parquet.Types.FixedLenByteArray, parquet.Encodings.Plain, false, col, memory.DefaultAllocator).(*PlainFixedLenByteArrayEncoder)
						values := make([]byte, n*width)
						entries := make([]parquet.FixedLenByteArray, n)
						for i := range entries {
							entries[i] = values[i*width : (i+1)*width]
						}
						enc.sink.Reserve(n * width)
						reset = func() { enc.sink.pos = 0 }
						release = enc.Release
						if path == "fixed-arrow" {
							encode = func() { enc.PutArrowSpaced(values, valid, bitOffset) }
						} else {
							encode = func() { enc.PutSpaced(entries, valid, bitOffset) }
						}
					} else {
						enc := NewEncoder(parquet.Types.ByteArray, parquet.Encodings.Plain, false, nil, memory.DefaultAllocator).(*PlainByteArrayEncoder)
						data := make([]byte, 11+n*width)
						offsets32 := make([]int32, n+1)
						offsets64 := make([]int64, n+1)
						entries := make([]parquet.ByteArray, n)
						for i := 0; i <= n; i++ {
							offsets32[i] = int32(11 + i*width)
							offsets64[i] = int64(11 + i*width)
							if i < n {
								entries[i] = data[11+i*width : 11+(i+1)*width]
							}
						}
						enc.sink.Reserve(n * (width + 4))
						reset = func() { enc.sink.pos = 0 }
						release = enc.Release
						switch path {
						case "arrow32":
							encode = func() { enc.PutArrowSpaced(data, offsets32, valid, bitOffset) }
						case "arrow64":
							encode = func() { enc.PutArrowSpaced64(data, offsets64, valid, bitOffset) }
						case "bytearray":
							encode = func() { enc.PutSpaced(entries, valid, bitOffset) }
						}
					}
					defer release()
					b.SetBytes(n * int64(width))
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						reset()
						encode()
					}
				})
			}
		}
	}
}
