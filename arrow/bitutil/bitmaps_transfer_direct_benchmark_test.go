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
	"strconv"
	"testing"
	"unsafe"
)

// transferBitmapBaseline is the implementation from upstream main before the
// direct-byte/word optimization. Keeping it here makes paired measurements
// possible on the same compiler and machine, without checking out two commits.
func transferBitmapBaseline(mode transferMode, src []byte, srcOffset, length int, dst []byte, dstOffset int) {
	if length == 0 {
		return
	}

	srcStart := srcOffset / 8
	dstStart := dstOffset / 8
	srcBytes := src[srcStart : srcStart+int(BytesForBits(int64(srcOffset%8)+int64(length)))]
	dstBytes := dst[dstStart : dstStart+int(BytesForBits(int64(dstOffset%8)+int64(length)))]
	srcPtr := uintptr(unsafe.Pointer(&srcBytes[0]))
	dstPtr := uintptr(unsafe.Pointer(&dstBytes[0]))
	var overlaps bool
	if srcPtr < dstPtr {
		overlaps = dstPtr-srcPtr < uintptr(len(srcBytes))
	} else {
		overlaps = srcPtr-dstPtr < uintptr(len(dstBytes))
	}
	if overlaps {
		src = bytes.Clone(srcBytes)
		srcOffset %= 8
	}

	bitOffset := srcOffset % 8
	destBitOffset := dstOffset % 8
	if bitOffset != 0 || destBitOffset != 0 {
		rdr := NewBitmapWordReader(src, srcOffset, length)
		wr := NewBitmapWordWriter(dst, dstOffset, length)

		nwords := rdr.Words()
		for nwords > 0 {
			nwords--
			if mode == transferInvert {
				wr.PutNextWord(^rdr.NextWord())
			} else {
				wr.PutNextWord(rdr.NextWord())
			}
		}
		nbytes := rdr.TrailingBytes()
		for nbytes > 0 {
			nbytes--
			bt, validBits := rdr.NextTrailingByte()
			if mode == transferInvert {
				bt = ^bt
			}
			wr.PutNextTrailingByte(bt, validBits)
		}
		return
	}

	nbytes := int(BytesForBits(int64(length)))
	src = src[srcOffset/8:]
	dst = dst[dstOffset/8:]
	trailingBits := nbytes*8 - length
	trailMask := byte(uint(1)<<(8-trailingBits)) - 1
	var lastData byte
	if mode == transferInvert {
		for i, b := range src[:nbytes-1] {
			dst[i] = ^b
		}
		lastData = ^src[nbytes-1]
	} else {
		copy(dst, src[:nbytes-1])
		lastData = src[nbytes-1]
	}

	dst[nbytes-1] &= ^trailMask
	dst[nbytes-1] |= lastData & trailMask
}

// BenchmarkTransferBitmapDirectOffsets compares the exact previous transfer
// path to the production function. Inputs and output buffers are reused.
func BenchmarkTransferBitmapDirectOffsets(b *testing.B) {
	operations := []struct {
		name string
		mode transferMode
	}{
		{"copy", transferCopy},
		{"invert", transferInvert},
	}
	offsets := []struct {
		name     string
		src, dst int
	}{
		{"aligned", 0, 0},
		{"same-3", 3, 3},
		{"same-7", 7, 7},
		{"shift-3-to-5", 3, 5},
		{"shift-7-to-0", 7, 0},
	}

	for _, operation := range operations {
		b.Run(operation.name, func(b *testing.B) {
			for _, nbits := range []int{64, 1024, 65536, 1048576} {
				b.Run(strconv.Itoa(nbits), func(b *testing.B) {
					for _, offset := range offsets {
						b.Run(offset.name, func(b *testing.B) {
							src := make([]byte, int(BytesForBits(int64(nbits+offset.src))))
							dst := make([]byte, int(BytesForBits(int64(nbits+offset.dst))))
							for i := range src {
								src[i] = byte(i*137 + 53)
							}
							for _, variant := range []string{"before", "after"} {
								b.Run(variant, func(b *testing.B) {
									b.SetBytes(int64(nbits / 8))
									b.ReportAllocs()
									if variant == "before" {
										for b.Loop() {
											transferBitmapBaseline(operation.mode, src, offset.src, nbits, dst, offset.dst)
										}
									} else {
										for b.Loop() {
											transferBitmap(operation.mode, src, offset.src, nbits, dst, offset.dst)
										}
									}
								})
							}
						})
					}
				})
			}
		})
	}
}
