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
	"sync"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestChunkIndexAndSlicesWithEmptyChunks(t *testing.T) {
	for _, count := range []int{32, 256, 1024} {
		t.Run(fmtChunkCount(count), func(t *testing.T) {
			mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
			defer mem.AssertSize(t, 0)

			builder := array.NewInt32Builder(mem)
			builder.AppendValues([]int32{10, 11, 12, 13}, []bool{true, false, true, true})
			values := builder.NewInt32Array()
			builder.Release()
			defer values.Release()

			empty := array.NewSlice(values, 0, 0)
			defer empty.Release()

			chunks := make([]arrow.Array, count)
			for i := range chunks {
				if i%7 == 0 {
					chunks[i] = empty
				} else {
					chunks[i] = values
				}
			}
			input := arrow.NewChunked(arrow.PrimitiveTypes.Int32, chunks)
			defer input.Release()

			// Exercise the first lazy index construction from concurrent readers.
			var wg sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					for j := 0; j < 200; j++ {
						position := (worker*103 + j*47) % input.Len()
						chunk, offset := input.ChunkIndex(position)
						if chunk < 0 || chunk >= len(chunks) || offset < 0 || offset >= chunks[chunk].Len() {
							t.Errorf("invalid chunk position %d: (%d, %d)", position, chunk, offset)
						}
					}
				}(worker)
			}
			wg.Wait()

			for position := 0; position < input.Len(); position++ {
				chunk, offset := input.ChunkIndex(position)
				wantedChunk, wantedOffset := 0, position
				for wantedOffset >= chunks[wantedChunk].Len() {
					wantedOffset -= chunks[wantedChunk].Len()
					wantedChunk++
				}
				if chunk != wantedChunk || offset != wantedOffset {
					t.Fatalf("position %d: got (%d,%d), want (%d,%d)",
						position, chunk, offset, wantedChunk, wantedOffset)
				}
			}
			atEnd, endOffset := input.ChunkIndex(input.Len())
			if atEnd != count || endOffset != 0 {
				t.Fatalf("end of array: got (%d,%d), want (%d,0)", atEnd, endOffset, count)
			}

			ranges := [][2]int{
				{0, 0}, {0, 1}, {0, input.Len()}, {input.Len(), input.Len()},
				{input.Len() - 1, input.Len()}, {input.Len() / 2, input.Len()/2 + 3},
				{4, input.Len() - 4},
			}
			for _, bounds := range ranges {
				start, end := bounds[0], bounds[1]
				out := array.NewChunkedSlice(input, int64(start), int64(end))
				if out.Len() != end-start {
					t.Errorf("slice [%d:%d] length: got %d", start, end, out.Len())
				}
				seen := 0
				for _, part := range out.Chunks() {
					arr := part.(*array.Int32)
					for i := 0; i < arr.Len(); i++ {
						slot := (start + seen + i) % 4
						if arr.IsValid(i) != (slot != 1) {
							t.Errorf("slice [%d:%d] row %d validity mismatch", start, end, seen+i)
						}
						if slot != 1 && arr.Value(i) != int32(10+slot) {
							t.Errorf("slice [%d:%d] row %d incorrect value", start, end, seen+i)
						}
					}
					seen += arr.Len()
				}
				if seen != end-start {
					t.Errorf("slice [%d:%d] consumed %d rows", start, end, seen)
				}
				out.Release()
			}
		})
	}
}

func fmtChunkCount(n int) string {
	switch n {
	case 32:
		return "small"
	case 256:
		return "threshold"
	default:
		return "many"
	}
}
