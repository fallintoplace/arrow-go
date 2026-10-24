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
	"bytes"
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

func plainSpacedBatchBitmap(n, offset int, pattern string) []byte {
	if pattern == "nil" {
		return nil
	}
	bitmap := make([]byte, bitutil.BytesForBits(int64(n+offset)))
	for i := 0; i < n; i++ {
		set := false
		switch pattern {
		case "all":
			set = true
		case "alternating":
			set = i%2 == 0
		case "sparse":
			set = i%8 == 1
		case "runs":
			set = i%128 < 64
		}
		if set {
			bitutil.SetBit(bitmap, offset+i)
		}
	}
	return bitmap
}

func plainSpacedBatchValid(bits []byte, offset, i int) bool {
	return bits == nil || bitutil.BitIsSet(bits, offset+i)
}

func plainSpacedBatchBytes(t *testing.T, enc TypedEncoder) []byte {
	t.Helper()
	buf, err := enc.FlushValues()
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Release()
	return append([]byte(nil), buf.Bytes()...)
}

func TestPlainSpacedBatchByteArrays(t *testing.T) {
	for _, n := range []int{0, 1, 31, 129, 4097} {
		// Include nonzero payload offsets and a sliced Arrow offsets buffer.
		data := []byte("leading payload ignored by slice")
		allOffsets32 := []int32{int32(len(data))}
		allOffsets64 := []int64{int64(len(data))}
		allValues := make([]parquet.ByteArray, n+6)
		for i := range allValues {
			sz := (i*13 + 7) % 73
			if i%17 == 0 {
				sz = 0
			}
			value := make([]byte, sz)
			for j := range value {
				value[j] = byte(i + j*7)
			}
			allValues[i] = value
			data = append(data, value...)
			allOffsets32 = append(allOffsets32, int32(len(data)))
			allOffsets64 = append(allOffsets64, int64(len(data)))
		}
		values := allValues[3 : n+3]
		offsets32 := allOffsets32[3 : n+4]
		offsets64 := allOffsets64[3 : n+4]

		for _, pattern := range []string{"nil", "none", "all", "alternating", "sparse", "runs"} {
			t.Run(fmt.Sprintf("rows=%d/%s", n, pattern), func(t *testing.T) {
				const offset = 3
				validity := plainSpacedBatchBitmap(n, offset, pattern)
				selected := make([]parquet.ByteArray, 0, n)
				for i, value := range values {
					if plainSpacedBatchValid(validity, offset, i) {
						selected = append(selected, value)
					}
				}
				sentinel := []parquet.ByteArray{[]byte("sentinel")}
				want := NewEncoder(parquet.Types.ByteArray, parquet.Encodings.Plain, false, nil, memory.DefaultAllocator).(*PlainByteArrayEncoder)
				defer want.Release()
				want.Put(sentinel)
				want.Put(selected)
				want.Put(sentinel)
				expected := plainSpacedBatchBytes(t, want)

				for _, kind := range []string{"values", "arrow32", "arrow64"} {
					t.Run(kind, func(t *testing.T) {
						enc := NewEncoder(parquet.Types.ByteArray, parquet.Encodings.Plain, false, nil, memory.DefaultAllocator).(*PlainByteArrayEncoder)
						defer enc.Release()
						enc.Put(sentinel)
						switch kind {
						case "values":
							enc.PutSpaced(values, validity, offset)
						case "arrow32":
							enc.PutArrowSpaced(data, offsets32, validity, offset)
						case "arrow64":
							enc.PutArrowSpaced64(data, offsets64, validity, offset)
						}
						enc.Put(sentinel)
						if actual := plainSpacedBatchBytes(t, enc); !bytes.Equal(actual, expected) {
							t.Fatalf("encoded bytes differ: got %d bytes, want %d bytes", len(actual), len(expected))
						}
					})
				}
			})
		}
	}
}

func TestPlainSpacedBatchFixedLength(t *testing.T) {
	for _, width := range []int{1, 16, 64} {
		col := schema.NewColumn(schema.NewFixedLenByteArrayNode("value", parquet.Repetitions.Required, int32(width), -1), 0, 0)
		for _, n := range []int{0, 1, 31, 129, 4097} {
			raw := make([]byte, n*width)
			values := make([]parquet.FixedLenByteArray, n)
			for i := 0; i < n; i++ {
				for j := range raw[i*width : (i+1)*width] {
					raw[i*width+j] = byte(i*3 + j*5)
				}
				values[i] = raw[i*width : (i+1)*width]
				if i%17 == 0 {
					clear(values[i])
					values[i] = nil // A valid nil value writes typeLen zero bytes.
				}
			}
			for _, pattern := range []string{"nil", "none", "all", "alternating", "sparse", "runs"} {
				t.Run(fmt.Sprintf("width=%d/rows=%d/%s", width, n, pattern), func(t *testing.T) {
					const offset = 3
					validity := plainSpacedBatchBitmap(n, offset, pattern)
					selected := make([]parquet.FixedLenByteArray, 0, n)
					for i, value := range values {
						if plainSpacedBatchValid(validity, offset, i) {
							selected = append(selected, value)
						}
					}
					sentinel := []parquet.FixedLenByteArray{bytes.Repeat([]byte{42}, width)}
					ref := NewEncoder(parquet.Types.FixedLenByteArray, parquet.Encodings.Plain, false, col, memory.DefaultAllocator).(*PlainFixedLenByteArrayEncoder)
					defer ref.Release()
					ref.Put(sentinel)
					ref.Put(selected)
					ref.Put(sentinel)
					expected := plainSpacedBatchBytes(t, ref)

					for _, kind := range []string{"values", "arrow"} {
						t.Run(kind, func(t *testing.T) {
							enc := NewEncoder(parquet.Types.FixedLenByteArray, parquet.Encodings.Plain, false, col, memory.DefaultAllocator).(*PlainFixedLenByteArrayEncoder)
							defer enc.Release()
							enc.Put(sentinel)
							switch kind {
							case "values":
								enc.PutSpaced(values, validity, offset)
							case "arrow":
								enc.PutArrowSpaced(raw, validity, offset)
							}
							enc.Put(sentinel)
							if actual := plainSpacedBatchBytes(t, enc); !bytes.Equal(actual, expected) {
								t.Fatalf("encoded bytes differ: got %d bytes, want %d bytes", len(actual), len(expected))
							}
						})
					}
				})
			}
		}
	}
}
