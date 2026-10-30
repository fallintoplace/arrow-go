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
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/internal/utils"
	"github.com/apache/arrow-go/v18/parquet"
)

func TestPlainFixedLenByteArrayDecoderBatchCursor(t *testing.T) {
	for _, width := range []int{1, 8, 32} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			const count = 11
			page := make([]byte, count*width+7)
			for i := range page {
				page[i] = byte(i*17 + 3)
			}
			dec := &PlainFixedLenByteArrayDecoder{decoder: decoder{typeLen: width}}
			if err := dec.SetData(count, page); err != nil {
				t.Fatal(err)
			}

			out := make([]parquet.FixedLenByteArray, count+3)
			if n, err := dec.Decode(nil); n != 0 || err != nil {
				t.Fatalf("empty decode: got %d, %v", n, err)
			}
			if n, err := dec.Decode(out[:3]); n != 3 || err != nil {
				t.Fatalf("first batch: got %d, %v", n, err)
			}
			if n, err := dec.Decode(out[3:]); n != count-3 || err != nil {
				t.Fatalf("second batch: got %d, %v", n, err)
			}
			if dec.ValuesLeft() != 0 || len(dec.data) != 7 {
				t.Fatalf("remaining: values=%d, bytes=%d", dec.ValuesLeft(), len(dec.data))
			}
			if n, err := dec.Decode(out); n != 0 || err != nil {
				t.Fatalf("exhausted decode: got %d, %v", n, err)
			}

			for i, value := range out[:count] {
				start := i * width
				if !bytes.Equal(value, page[start:start+width]) {
					t.Fatalf("value %d differs", i)
				}
				// PLAIN output aliases the page and retains its original slice capacity.
				if cap(value) != cap(page)-start {
					t.Fatalf("value %d: cap=%d, want %d", i, cap(value), cap(page)-start)
				}
			}
			for i, value := range out[count:] {
				if value != nil {
					t.Fatalf("output beyond decoded values at %d was modified", count+i)
				}
			}
			out[0][0] ^= 0xff
			if out[0][0] != page[0] {
				t.Fatal("decoded value no longer aliases the page")
			}
		})
	}
}

func TestPlainFixedLenByteArrayDecoderBatchCursorTruncated(t *testing.T) {
	const width = 8
	dec := &PlainFixedLenByteArrayDecoder{decoder: decoder{typeLen: width}}
	data := make([]byte, width*2)
	if err := dec.SetData(3, data); err != nil {
		t.Fatal(err)
	}
	out := make([]parquet.FixedLenByteArray, 3)
	out[0] = parquet.FixedLenByteArray{42}
	n, err := dec.Decode(out)
	if n != 0 || err == nil || err.Error() != "parquet: eof exception" {
		t.Fatalf("truncated decode: got %d, %v", n, err)
	}
	if dec.ValuesLeft() != 3 || len(dec.data) != len(data) {
		t.Fatal("failed decode advanced the cursor")
	}
	if !bytes.Equal(out[0], []byte{42}) || out[1] != nil || out[2] != nil {
		t.Fatal("failed decode modified output")
	}
}

// The reference is the previous implementation, kept in the benchmark so that
// before/after measurements use the same binary, inputs and Go toolchain.
func decodePlainFixedLenByteArrayBefore(dec *PlainFixedLenByteArrayDecoder, out []parquet.FixedLenByteArray) (int, error) {
	max := utils.Min(len(out), dec.nvals)
	numBytesNeeded := max * dec.typeLen
	if numBytesNeeded > len(dec.data) || numBytesNeeded > math.MaxInt32 {
		return 0, errors.New("parquet: eof exception")
	}

	for idx := range out[:max] {
		out[idx] = dec.data[:dec.typeLen]
		dec.data = dec.data[dec.typeLen:]
	}

	dec.nvals -= max
	return max, nil
}

func BenchmarkPlainFixedLenByteArrayDecoderBatchCursor(b *testing.B) {
	for _, width := range []int{8, 32} {
		for _, count := range []int{64, 1024, 65536} {
			data := make([]byte, count*width)
			out := make([]parquet.FixedLenByteArray, count)
			for _, variant := range []struct {
				name   string
				decode func(*PlainFixedLenByteArrayDecoder, []parquet.FixedLenByteArray) (int, error)
			}{
				{"before", decodePlainFixedLenByteArrayBefore},
				{"after", (*PlainFixedLenByteArrayDecoder).Decode},
			} {
				b.Run(fmt.Sprintf("width=%d/values=%d/%s", width, count, variant.name), func(b *testing.B) {
				dec := &PlainFixedLenByteArrayDecoder{decoder: decoder{typeLen: width}}
				b.ReportAllocs()
				b.SetBytes(int64(len(data)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dec.data = data
					dec.nvals = count
					if _, err := variant.decode(dec, out); err != nil {
						b.Fatal(err)
					}
				}
			})
			}
		}
	}
}
