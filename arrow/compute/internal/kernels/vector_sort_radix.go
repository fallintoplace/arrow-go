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

//go:build go1.22

package kernels

import (
	"unsafe"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute/exec"
)

const minIntegerRadixSortLength = 2048

type radixInteger interface {
	~int8 | ~uint8 | ~int16 | ~uint16 | ~int32 | ~uint32 | ~int64 | ~uint64
}

// tryIntegerRadixSort handles large, null-free, single-chunk integer arrays.
// The caller falls back to the existing stable comparator sort when this returns false.
func tryIntegerRadixSort(ctx *exec.KernelCtx, arr arrow.Array, indices []uint64, order SortOrder) bool {
	switch v := arr.(type) {
	case *array.Int8:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Int16:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Int32:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Int64:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Uint8:
		return radixSortIntegerValues(ctx, v.Values(), indices, false, order)
	case *array.Uint16:
		return radixSortIntegerValues(ctx, v.Values(), indices, false, order)
	case *array.Uint32:
		return radixSortIntegerValues(ctx, v.Values(), indices, false, order)
	case *array.Uint64:
		return radixSortIntegerValues(ctx, v.Values(), indices, false, order)
	case *array.Date32:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Date64:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Time32:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Time64:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Timestamp:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	case *array.Duration:
		return radixSortIntegerValues(ctx, v.Values(), indices, true, order)
	default:
		return false
	}
}

// radixSortIntegerValues is a stable LSD byte radix sort of row indices.
// XOR of the sign bit maps signed integers to unsigned comparison order;
// complementing the key reverses the order without breaking stability.
func radixSortIntegerValues[T radixInteger](ctx *exec.KernelCtx, values []T, indices []uint64, signed bool, order SortOrder) bool {
	if len(indices) < minIntegerRadixSortLength {
		return false
	}

	// A comparator stable sort handles reverse-sorted input efficiently.
	// Already-ordered input needs no sort or scratch allocation.
	sorted, reversed := true, true
	for i := 1; i < len(values); i++ {
		prev, next := values[i-1], values[i]
		if order == Descending {
			if prev < next {
				sorted = false
			}
			if prev > next {
				reversed = false
			}
		} else {
			if prev > next {
				sorted = false
			}
			if prev < next {
				reversed = false
			}
		}
		if !sorted && !reversed {
			break
		}
	}
	if sorted {
		return true
	}
	if reversed {
		return false
	}

	width := int(unsafe.Sizeof(values[0]))
	bitWidth := width * 8
	mask := ^uint64(0) >> (64 - bitWidth)
	var xorMask uint64
	if signed {
		xorMask = uint64(1) << (bitWidth - 1)
	}
	if order == Descending {
		xorMask ^= mask
	}

	scratchBuf := ctx.Allocate(len(indices) * arrow.Uint64SizeBytes)
	defer scratchBuf.Release()
	scratch := arrow.GetData[uint64](scratchBuf.Buf())[:len(indices)]

	src, dst := indices, scratch
	inScratch := false
	for shift := uint(0); shift < uint(bitWidth); shift += 8 {
		var counts [256]int
		for _, row := range src {
			digit := byte(((uint64(values[row]) ^ xorMask) & mask) >> shift)
			counts[digit]++
		}

		// Skip byte passes where every key has the same byte (common for narrow ranges).
		allSame := false
		for _, count := range counts {
			if count == len(src) {
				allSame = true
				break
			}
		}
		if allSame {
			continue
		}

		pos := 0
		for i, count := range counts {
			counts[i] = pos
			pos += count
		}
		for _, row := range src {
			digit := byte(((uint64(values[row]) ^ xorMask) & mask) >> shift)
			dst[counts[digit]] = row
			counts[digit]++
		}
		src, dst = dst, src
		inScratch = !inScratch
	}
	if inScratch {
		copy(indices, src)
	}
	return true
}
