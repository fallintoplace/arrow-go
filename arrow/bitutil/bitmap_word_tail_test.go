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
	"testing"
)

func TestBitmapWordTrailingByteRead(t *testing.T) {
	for offset := 0; offset < 8; offset++ {
		for validBits := 1; validBits <= 8; validBits++ {
			for _, first := range []byte{0, 0xff, 0x55, 0xaa, 0x81, 0x42} {
				for _, second := range []byte{0, 0xff, 0x55, 0xaa, 0x81, 0x42} {
					src := []byte{first, second}
					reader := NewBitmapWordReader(src, offset, validBits)
					value, n := reader.NextTrailingByte()
					var want byte
					for i := 0; i < validBits; i++ {
						if BitIsSet(src, offset+i) {
							want |= 1 << uint(i)
						}
					}
					if n != validBits || value != want {
						t.Fatalf("offset=%d validBits=%d src=%x: got (%x, %d), want (%x, %d)",
							offset, validBits, src, value, n, want, validBits)
					}
				}
			}
		}
	}
}

func TestBitmapWordTrailingByteWrite(t *testing.T) {
	for offset := 0; offset < 8; offset++ {
		for validBits := 1; validBits < 8; validBits++ {
			for _, value := range []byte{0, 0xff, 0x55, 0xaa, 0x81, 0x42} {
				for _, first := range []byte{0, 0xff, 0x55, 0xaa} {
					dst := []byte{first, first ^ 0x55}
					want := bytes.Clone(dst)
					for i := 0; i < validBits; i++ {
						SetBitTo(want, offset+i, value&(1<<uint(i)) != 0)
					}
					writer := NewBitmapWordWriter(dst, offset, validBits)
					writer.PutNextTrailingByte(value, validBits)
					if !bytes.Equal(dst, want) {
						t.Fatalf("offset=%d validBits=%d value=%x: got %x, want %x",
								offset, validBits, value, dst, want)
					}
				}
			}
		}
	}
}

func TestBitmapWordTrailingByteRoundTrip(t *testing.T) {
	for srcOffset := 0; srcOffset < 8; srcOffset++ {
		for dstOffset := 0; dstOffset < 8; dstOffset++ {
			for length := 1; length <= 160; length++ {
				src := make([]byte, 24)
				dst := make([]byte, 24)
				for i := range src {
					src[i] = byte(i*73 + 0x4b)
					dst[i] = byte(i*47 + 0x9d)
				}
				want := bytes.Clone(dst)
				for i := 0; i < length; i++ {
					SetBitTo(want, dstOffset+i, BitIsSet(src, srcOffset+i))
				}
				reader := NewBitmapWordReader(src, srcOffset, length)
				writer := NewBitmapWordWriter(dst, dstOffset, length)
				for n := reader.Words(); n > 0; n-- {
					writer.PutNextWord(reader.NextWord())
				}
				for n := reader.TrailingBytes(); n > 0; n-- {
					value, validBits := reader.NextTrailingByte()
					writer.PutNextTrailingByte(value, validBits)
				}
				if !bytes.Equal(dst, want) {
					t.Fatalf("srcOffset=%d dstOffset=%d length=%d: got %x, want %x",
						srcOffset, dstOffset, length, dst, want)
				}
			}
		}
	}
}

var bitmapWordTailBenchmarkSink byte

func BenchmarkBitmapWordTrailingByteRead(b *testing.B) {
	for _, offset := range []int{0, 3, 7} {
		for _, validBits := range []int{1, 3, 7, 8} {
			b.Run(fmt.Sprintf("offset%d_bits%d", offset, validBits), func(b *testing.B) {
				src := []byte{0xa7, 0xd3}
				reader := BitmapWordReader{bitmap: src, offset: offset, trailingBits: validBits}
				var result byte
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					reader.trailingBits = validBits
					value, _ := reader.NextTrailingByte()
					result ^= value
				}
				bitmapWordTailBenchmarkSink = result
			})
		}
	}
}

func BenchmarkBitmapWordTrailingByteWrite(b *testing.B) {
	for _, offset := range []int{0, 3, 7} {
		for _, validBits := range []int{1, 3, 7} {
			b.Run(fmt.Sprintf("offset%d_bits%d", offset, validBits), func(b *testing.B) {
				dst := []byte{0x5b, 0xca}
				writer := BitmapWordWriter{bitmap: dst, offset: offset}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					writer.PutNextTrailingByte(byte(i*131), validBits)
				}
				bitmapWordTailBenchmarkSink = dst[0] ^ dst[1]
			})
		}
	}
}
