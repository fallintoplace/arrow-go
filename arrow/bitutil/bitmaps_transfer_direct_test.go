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

func TestTransferBitmapDirectOffsets(t *testing.T) {
	operations := []struct {
		name   string
		invert bool
		run    func([]byte, int, int, []byte, int)
	}{
		{"copy", false, bitutil.CopyBitmap},
		{"invert", true, bitutil.InvertBitmap},
	}
	layouts := []struct {
		name               string
		separate           bool
		srcSlice, dstSlice int
	}{
		{name: "separate", separate: true},
		{name: "same-slice"},
		{name: "source-before-dest", dstSlice: 1},
		{name: "dest-before-source", srcSlice: 1},
		{name: "overlap-forward", srcSlice: 2, dstSlice: 9},
		{name: "overlap-backward", srcSlice: 9, dstSlice: 2},
	}
	lengths := []int{0, 1, 2, 3, 4, 7, 8, 9, 15, 16, 17, 31, 32, 63, 64, 65, 66, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1024, 1025, 4096, 4099}
	random := rand.New(rand.NewSource(12345))

	for _, operation := range operations {
		for _, length := range lengths {
			for srcOffset := 0; srcOffset < 24; srcOffset++ {
				for dstOffset := 0; dstOffset < 24; dstOffset++ {
					for _, layout := range layouts {
						var src, dst, wantSrc, wantDst, backing, wantBacking []byte
						if layout.separate {
							src = make([]byte, bitutil.BytesForBits(int64(srcOffset+length)))
							dst = make([]byte, bitutil.BytesForBits(int64(dstOffset+length)))
							random.Read(src)
							random.Read(dst)
							wantSrc = bytes.Clone(src)
							wantDst = bytes.Clone(dst)
						} else {
							backing = make([]byte, bitutil.BytesForBits(int64(max(srcOffset, dstOffset)+length))+16)
							random.Read(backing)
							wantBacking = bytes.Clone(backing)
							src = backing[layout.srcSlice:]
							dst = backing[layout.dstSlice:]
							wantSrc = wantBacking[layout.srcSlice:]
							wantDst = wantBacking[layout.dstSlice:]
						}

						// Read from the original bitmap, not the destination
						// as it is modified. This is also the overlap oracle.
						snapshot := bytes.Clone(wantSrc)
						for i := 0; i < length; i++ {
							value := bitutil.BitIsSet(snapshot, srcOffset+i)
							if operation.invert {
								value = !value
							}
							bitutil.SetBitTo(wantDst, dstOffset+i, value)
						}
						operation.run(src, srcOffset, length, dst, dstOffset)
						if !bytes.Equal(dst, wantDst) {
							t.Fatalf("%s len=%d srcOffset=%d dstOffset=%d layout=%s: destination differs",
								operation.name, length, srcOffset, dstOffset, layout.name)
						}
						if layout.separate {
							if !bytes.Equal(src, snapshot) {
								t.Fatalf("%s len=%d srcOffset=%d dstOffset=%d: source modified",
									operation.name, length, srcOffset, dstOffset)
							}
						} else if !bytes.Equal(backing, wantBacking) {
							t.Fatalf("%s len=%d srcOffset=%d dstOffset=%d layout=%s: bits outside destination changed",
								operation.name, length, srcOffset, dstOffset, layout.name)
						}
					}
				}
			}
		}
	}
}
