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
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

func TestBitmapWordReaderTrailingByte(t *testing.T) {
	patterns := [][3]byte{
		{0x00, 0x00, 0x00},
		{0xff, 0xff, 0xff},
		{0xa5, 0x5a, 0xc3},
		{0x5a, 0xa5, 0x3c},
	}
	for offset := 0; offset < 16; offset++ {
		for length := 1; length <= 8; length++ {
			for _, pattern := range patterns {
				reader := bitutil.NewBitmapWordReader(pattern[:], offset, length)
				got, validBits := reader.NextTrailingByte()
				var want byte
				for i := 0; i < length; i++ {
					if bitutil.BitIsSet(pattern[:], offset+i) {
						want |= 1 << i
					}
				}
				if got != want || validBits != length {
					t.Fatalf("offset=%d length=%d bitmap=%x: got (%08b, %d), want (%08b, %d)",
						offset, length, pattern, got, validBits, want, length)
				}
			}
		}
	}
}

func TestBitmapWordWriterTrailingByte(t *testing.T) {
	values := []byte{0x00, 0xff, 0xa5, 0x5a, 0x81}
	for offset := 0; offset < 16; offset++ {
		for length := 1; length < 8; length++ {
			for _, initial := range []byte{0x00, 0xff, 0xa5, 0x5a} {
				for _, value := range values {
					got := bytes.Repeat([]byte{initial}, 4)
					want := bytes.Clone(got)
					writer := bitutil.NewBitmapWordWriter(got, offset, length)
					writer.PutNextTrailingByte(value, length)
					for i := 0; i < length; i++ {
						bitutil.SetBitTo(want, offset+i, (value>>i)&1 != 0)
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("offset=%d length=%d initial=%02x value=%02x: got %x, want %x",
							offset, length, initial, value, got, want)
					}
				}
			}
		}
	}
}

func TestBitmapWordTrailingBytesRoundTrip(t *testing.T) {
	for offset := 0; offset < 16; offset++ {
		for length := 1; length <= 264; length++ {
			src := make([]byte, (offset+length+7)/8)
			for i := range src {
				src[i] = byte(i*37 + offset*11 + length*13)
			}
			got := make([]byte, len(src))
			for i := range got {
				got[i] = 0xa5
			}
			want := bytes.Clone(got)
			for i := 0; i < length; i++ {
				bitutil.SetBitTo(want, offset+i, bitutil.BitIsSet(src, offset+i))
			}

			reader := bitutil.NewBitmapWordReader(src, offset, length)
			writer := bitutil.NewBitmapWordWriter(got, offset, length)
			for i, nwords := 0, reader.Words(); i < nwords; i++ {
				writer.PutNextWord(reader.NextWord())
			}
			for i, nbytes := 0, reader.TrailingBytes(); i < nbytes; i++ {
				value, validBits := reader.NextTrailingByte()
				writer.PutNextTrailingByte(value, validBits)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("offset=%d length=%d: got %x, want %x", offset, length, got, want)
			}
		}
	}
}

var bitmapWordTrailingSink byte

func BenchmarkBitmapWordReaderTrailingByte(b *testing.B) {
	for _, tc := range []struct{ offset, length int }{
		{0, 1}, {0, 7}, {0, 8}, {3, 4}, {3, 8}, {7, 1}, {7, 7}, {7, 8},
	} {
		b.Run(fmt.Sprintf("offset%d/bits%d", tc.offset, tc.length), func(b *testing.B) {
			bitmap := []byte{0xb5, 0x4a}
			b.ReportAllocs()
			var value byte
			for i := 0; i < b.N; i++ {
				reader := bitutil.NewBitmapWordReader(bitmap, tc.offset, tc.length)
				value, _ = reader.NextTrailingByte()
			}
			bitmapWordTrailingSink = value
		})
	}
}

func BenchmarkBitmapWordWriterTrailingByte(b *testing.B) {
	for _, tc := range []struct{ offset, length int }{
		{0, 1}, {0, 7}, {3, 4}, {3, 7}, {7, 1}, {7, 7},
	} {
		b.Run(fmt.Sprintf("offset%d/bits%d", tc.offset, tc.length), func(b *testing.B) {
			var bitmap [2]byte
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bitmap[0], bitmap[1] = 0xb5, 0x4a
				writer := bitutil.NewBitmapWordWriter(bitmap[:], tc.offset, tc.length)
				writer.PutNextTrailingByte(0xa5, tc.length)
			}
			bitmapWordTrailingSink = bitmap[0] ^ bitmap[1]
		})
	}
}
