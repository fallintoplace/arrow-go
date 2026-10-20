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
	"reflect"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute/exec"
)

const radixSortMinRows = 2048

type radixInteger interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// integerRadixSortable only accepts null-free physical integer arrays with the
// same concrete Go type in every chunk. Other types retain the comparator path.
func integerRadixSortable(chunks []arrow.Array) bool {
	if len(chunks) == 0 {
		return false
	}
	switch chunks[0].(type) {
	case *array.Int8, *array.Int16, *array.Int32, *array.Int64,
		*array.Uint8, *array.Uint16, *array.Uint32, *array.Uint64:
	default:
		return false
	}
	t := reflect.TypeOf(chunks[0])
	for _, chunk := range chunks {
		if chunk.NullN() != 0 || reflect.TypeOf(chunk) != t {
			return false
		}
	}
	return true
}

// sortIntegerRadixIndices bypasses comparator and per-row chunk-map setup for
// large integer columns. The existing comparison path handles everything else.
func sortIntegerRadixIndices(ctx *exec.KernelCtx, chunks []arrow.Array, nRows int, order SortOrder) *exec.ExecResult {
	buf := ctx.Allocate(nRows * arrow.Uint64SizeBytes)
	indices := arrow.GetData[uint64](buf.Buf())[:nRows]
	for i := range indices {
		indices[i] = uint64(i)
	}

	tmpBuf := ctx.Allocate(nRows * arrow.Uint64SizeBytes)
	tmp := arrow.GetData[uint64](tmpBuf.Buf())[:nRows]
	radixSortIntegerChunks(indices, tmp, chunks, order)
	tmpBuf.Release()

	out := &exec.ExecResult{Len: int64(nRows), Type: arrow.PrimitiveTypes.Uint64}
	out.Buffers[1].WrapBuffer(buf)
	return out
}

func radixSortIntegerChunks(indices, scratch []uint64, chunks []arrow.Array, order SortOrder) {
	descending := order == Descending
	switch chunks[0].(type) {
	case *array.Int8:
		radixSortIntegerValues[int8](indices, scratch, chunks, 8, true, descending)
	case *array.Int16:
		radixSortIntegerValues[int16](indices, scratch, chunks, 16, true, descending)
	case *array.Int32:
		radixSortIntegerValues[int32](indices, scratch, chunks, 32, true, descending)
	case *array.Int64:
		radixSortIntegerValues[int64](indices, scratch, chunks, 64, true, descending)
	case *array.Uint8:
		radixSortIntegerValues[uint8](indices, scratch, chunks, 8, false, descending)
	case *array.Uint16:
		radixSortIntegerValues[uint16](indices, scratch, chunks, 16, false, descending)
	case *array.Uint32:
		radixSortIntegerValues[uint32](indices, scratch, chunks, 32, false, descending)
	case *array.Uint64:
		radixSortIntegerValues[uint64](indices, scratch, chunks, 64, false, descending)
	}
}

func radixSortIntegerValues[T radixInteger](indices, scratch []uint64, chunks []arrow.Array, width uint, signed, descending bool) {
	var values []T
	if len(chunks) == 1 {
		values = chunks[0].(interface{ Values() []T }).Values()
	} else {
		// Flatten only the values, not the Arrow arrays or their validity buffers.
		// This also avoids the random-access chunk resolver during every radix pass.
		values = make([]T, 0, len(indices))
		for _, chunk := range chunks {
			values = append(values, chunk.(interface{ Values() []T }).Values()...)
		}
	}

	valueMask := ^uint64(0) >> (64 - width)
	xorMask := uint64(0)
	if signed {
		xorMask = uint64(1) << (width - 1)
	}
	if descending {
		xorMask ^= valueMask
	}

	// Check for sorted data and find constant bytes. Both let us skip passes
	// without changing the order of equal keys.
	first := (uint64(values[indices[0]]) & valueMask) ^ xorMask
	prev := first
	var varying uint64
	sorted := true
	for _, idx := range indices[1:] {
		key := (uint64(values[idx]) & valueMask) ^ xorMask
		varying |= key ^ first
		if key < prev {
			sorted = false
		}
		prev = key
	}
	if sorted {
		return
	}

	from, to := indices, scratch
	inScratch := false
	for shift := uint(0); shift < width; shift += 8 {
		if byte(varying>>shift) == 0 {
			continue
		}

		var counts [256]int
		for _, idx := range from {
			key := (uint64(values[idx]) & valueMask) ^ xorMask
			counts[byte(key>>shift)]++
		}

		position := 0
		for i, count := range counts {
			counts[i] = position
			position += count
		}
		// Forward iteration and prefix sums make every pass stable.
		for _, idx := range from {
			key := (uint64(values[idx]) & valueMask) ^ xorMask
			bucket := byte(key >> shift)
			to[counts[bucket]] = idx
			counts[bucket]++
		}
		from, to = to, from
		inScratch = !inScratch
	}
	if inScratch {
		copy(indices, from)
	}
}
