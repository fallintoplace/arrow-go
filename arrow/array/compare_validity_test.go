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

package array_test

import (
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/bitutil"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/assert"
)

func makeInt32ArrayWithValidity(t testing.TB, mem memory.Allocator, values []int32, validity []byte, length, offset int) arrow.Array {
	t.Helper()

	valueBytes := mem.Allocate(arrow.Int32Traits.BytesRequired(len(values)))
	copy(valueBytes, arrow.Int32Traits.CastToBytes(values))
	valueBuffer := memory.NewBufferWithAllocator(valueBytes, mem)
	var validityBuffer *memory.Buffer
	if validity != nil {
		validityBytes := mem.Allocate(len(validity))
		copy(validityBytes, validity)
		validityBuffer = memory.NewBufferWithAllocator(validityBytes, mem)
	}

	nulls := 0
	for i := offset; i < offset+length; i++ {
		if validity != nil && bitutil.BitIsNotSet(validity, i) {
			nulls++
		}
	}

	data := array.NewData(
		arrow.PrimitiveTypes.Int32,
		length,
		[]*memory.Buffer{validityBuffer, valueBuffer},
		nil,
		nulls,
		offset,
	)
	out := array.NewInt32Data(data)
	data.Release()
	valueBuffer.Release()
	if validityBuffer != nil {
		validityBuffer.Release()
	}
	return out
}

func setValidityBits(bitmap []byte, offset int, values []bool) {
	for i, value := range values {
		bitutil.SetBitTo(bitmap, offset+i, value)
	}
}

func TestArrayEqualValidityBitmaps(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.NewGoAllocator())
	defer mem.AssertSize(t, 0)

	tests := []struct {
		name        string
		values      []int32
		leftBitmap  []byte
		rightBitmap []byte
		length      int
		offset      int
		want        bool
	}{
		{
			name:        "missing bitmaps",
			values:      []int32{1, 2, 3, 4},
			length:      4,
			leftBitmap:  nil,
			rightBitmap: nil,
			want:        true,
		},
		{
			name:        "all valid bitmap and missing bitmap",
			values:      []int32{1, 2, 3, 4, 5, 6, 7, 8},
			leftBitmap:  nil,
			rightBitmap: []byte{0xff},
			length:      8,
			want:        true,
		},
		{
			name:   "same bits with non-byte-aligned offset",
			values: []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			leftBitmap: func() []byte {
				bitmap := []byte{0xa5, 0x5a}
				setValidityBits(bitmap, 3, []bool{true, false, true, true, false, true, true})
				return bitmap
			}(),
			rightBitmap: func() []byte {
				bitmap := []byte{0x1b, 0xe1}
				setValidityBits(bitmap, 3, []bool{true, false, true, true, false, true, true})
				return bitmap
			}(),
			length: 7,
			offset: 3,
			want:   true,
		},
		{
			name:   "different bits with the same null count",
			values: []int32{0, 1, 2, 3, 4, 5, 6, 7},
			leftBitmap: func() []byte {
				bitmap := []byte{0}
				setValidityBits(bitmap, 0, []bool{true, false, true, false, true, true, false, false})
				return bitmap
			}(),
			rightBitmap: func() []byte {
				bitmap := []byte{0}
				setValidityBits(bitmap, 0, []bool{false, true, true, false, true, true, false, false})
				return bitmap
			}(),
			length: 8,
			want:   false,
		},
		{
			name:        "empty arrays ignore bitmap contents",
			values:      []int32{0, 1, 2, 3, 4, 5, 6, 7, 8},
			leftBitmap:  []byte{0x00, 0xff},
			rightBitmap: []byte{0xff, 0x00},
			length:      0,
			offset:      8,
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			left := makeInt32ArrayWithValidity(t, mem, tc.values, tc.leftBitmap, tc.length, tc.offset)
			right := makeInt32ArrayWithValidity(t, mem, tc.values, tc.rightBitmap, tc.length, tc.offset)
			assert.Equal(t, tc.want, array.Equal(left, right))
			left.Release()
			right.Release()
		})
	}

	t.Run("same bits with different offsets across words", func(t *testing.T) {
		const (
			length      = 70
			leftOffset  = 3
			rightOffset = 5
		)

		leftValues := make([]int32, leftOffset+length)
		rightValues := make([]int32, rightOffset+length)
		for i := 0; i < length; i++ {
			leftValues[leftOffset+i] = int32(i)
			rightValues[rightOffset+i] = int32(i)
		}

		leftBitmap := make([]byte, bitutil.BytesForBits(leftOffset+length))
		rightBitmap := make([]byte, bitutil.BytesForBits(rightOffset+length))
		for i := 0; i < length; i++ {
			valid := i%7 != 0
			bitutil.SetBitTo(leftBitmap, leftOffset+i, valid)
			bitutil.SetBitTo(rightBitmap, rightOffset+i, valid)
		}

		left := makeInt32ArrayWithValidity(t, mem, leftValues, leftBitmap, length, leftOffset)
		right := makeInt32ArrayWithValidity(t, mem, rightValues, rightBitmap, length, rightOffset)
		assert.True(t, array.Equal(left, right))
		left.Release()
		right.Release()
	})
}

func BenchmarkArrayEqualValidityBitmap(b *testing.B) {
	const length = 1 << 20

	values := make([]int32, length)
	validity := make([]byte, bitutil.BytesForBits(length))
	for i := range length {
		if i%100 != 0 {
			bitutil.SetBit(validity, i)
		}
	}

	tests := []struct {
		name     string
		validity []byte
	}{
		{name: "all-valid/missing-bitmap"},
		{name: "1pct-null/bitmap", validity: validity},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			left := makeInt32ArrayWithValidity(b, memory.DefaultAllocator, values, tc.validity, length, 0)
			right := makeInt32ArrayWithValidity(b, memory.DefaultAllocator, values, tc.validity, length, 0)
			defer left.Release()
			defer right.Release()

			b.SetBytes(int64(length * 4))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if !array.Equal(left, right) {
					b.Fatal("equal arrays compared unequal")
				}
			}
		})
	}
}
