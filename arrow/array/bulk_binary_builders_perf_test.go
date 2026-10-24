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

package array

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func assertBulkBuilderSameArray(t *testing.T, got, want arrow.Array) {
	t.Helper()
	defer got.Release()
	defer want.Release()
	if !Equal(got, want) {
		t.Fatalf("arrays differ: got=%s want=%s", got, want)
	}
	gotBuffers, wantBuffers := got.Data().Buffers(), want.Data().Buffers()
	if len(gotBuffers) != len(wantBuffers) {
		t.Fatalf("buffer count differs: %d vs %d", len(gotBuffers), len(wantBuffers))
	}
	for i := range gotBuffers {
		if gotBuffers[i] == nil || wantBuffers[i] == nil {
			if gotBuffers[i] != wantBuffers[i] {
				t.Fatalf("nil buffer differs at %d", i)
			}
			continue
		}
		if !bytes.Equal(gotBuffers[i].Bytes(), wantBuffers[i].Bytes()) {
			t.Fatalf("raw buffer %d differs", i)
		}
	}
}

func TestBulkBinaryBuilderMatchesScalarAppends(t *testing.T) {
	values := [][]byte{
		[]byte("one"), nil, []byte("hello"), []byte(""), []byte("world"),
		[]byte{0, 1, 0, 255}, []byte("long payload long payload"), nil, []byte("tail"),
	}
	strings := make([]string, len(values))
	for i, v := range values {
		strings[i] = string(v)
	}
	valid := []bool{true, false, true, true, false, true, true, false, true}
	for _, dtype := range []arrow.BinaryDataType{arrow.BinaryTypes.Binary, arrow.BinaryTypes.LargeBinary} {
		for _, useStrings := range []bool{false, true} {
			for _, withNulls := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/strings=%t/nulls=%t", dtype, useStrings, withNulls), func(t *testing.T) {
					mem := memory.NewCheckedAllocator(memory.NewGoAllocator())
					defer mem.AssertSize(t, 0)
					bulk := NewBinaryBuilder(mem, dtype)
					scalar := NewBinaryBuilder(mem, dtype)
					defer bulk.Release()
					defer scalar.Release()
					bulk.AppendString("prefix")
					scalar.AppendString("prefix")
					var mask []bool
					if withNulls {
						mask = valid
					}
					for j := 0; j < 2; j++ {
						if useStrings {
							bulk.AppendStringValues(strings, mask)
						} else {
							bulk.AppendValues(values, mask)
						}
						for i, value := range values {
							scalar.Append(value)
							if len(mask) > 0 && !mask[i] {
								scalar.SetNull(scalar.Len() - 1)
							}
						}
					}
					bulk.AppendString("suffix")
					scalar.AppendString("suffix")
					assertBulkBuilderSameArray(t, bulk.NewArray(), scalar.NewArray())
				})
			}
		}
	}
}

func TestBulkFixedSizeBinaryBuilderMatchesScalarAppends(t *testing.T) {
	for _, width := range []int{1, 8, 32, 64} {
		for _, withNulls := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/nulls=%t", width, withNulls), func(t *testing.T) {
				mem := memory.NewCheckedAllocator(memory.NewGoAllocator())
				defer mem.AssertSize(t, 0)
				dtype := &arrow.FixedSizeBinaryType{ByteWidth: width}
				bulk := NewFixedSizeBinaryBuilder(mem, dtype)
				scalar := NewFixedSizeBinaryBuilder(mem, dtype)
				defer bulk.Release()
				defer scalar.Release()
				bulk.Append(bytes.Repeat([]byte{42}, width))
				scalar.Append(bytes.Repeat([]byte{42}, width))
				values := make([][]byte, 19)
				mask := make([]bool, len(values))
				for i := range values {
					mask[i] = i%4 != 2
					if i%5 != 0 {
						values[i] = bytes.Repeat([]byte{byte(i + 1)}, width)
					}
				}
				if !withNulls {
					mask = nil
				}
				for j := 0; j < 2; j++ {
					bulk.AppendValues(values, mask)
					for i, value := range values {
						if len(value) == 0 {
							scalar.AppendEmptyValue()
						} else {
							scalar.Append(value)
						}
						if len(mask) > 0 && !mask[i] {
							scalar.SetNull(scalar.Len() - 1)
						}
					}
				}
				assertBulkBuilderSameArray(t, bulk.NewArray(), scalar.NewArray())
			})
		}
	}
}

func BenchmarkBulkBinaryBuilders(b *testing.B) {
	allocator := memory.NewGoAllocator()
	for _, n := range []int{128, 4096, 65536} {
		binaryValues := make([][]byte, n)
		stringValues := make([]string, n)
		fixedValues := make([][]byte, n)
		mask := make([]bool, n)
		for i := range binaryValues {
			// Vary the payload length, including empty values.
			binaryValues[i] = bytes.Repeat([]byte{byte(i)}, i%31)
			stringValues[i] = string(binaryValues[i])
			if i%7 != 0 {
				fixedValues[i] = bytes.Repeat([]byte{byte(i)}, 32)
			}
			mask[i] = i%5 != 0
		}
		for _, mode := range []string{"all_valid", "mixed_nulls"} {
			var valid []bool
			if mode == "mixed_nulls" {
				valid = mask
			}
			for _, kind := range []string{"binary", "large_binary", "strings", "fixed32"} {
				b.Run(fmt.Sprintf("%s/%d/%s", kind, n, mode), func(b *testing.B) {
					b.SetBytes(int64(n * 16))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						switch kind {
						case "binary", "large_binary":
							dtype := arrow.BinaryTypes.Binary
							if kind == "large_binary" {
								dtype = arrow.BinaryTypes.LargeBinary
							}
							builder := NewBinaryBuilder(allocator, dtype)
							builder.AppendValues(binaryValues, valid)
							array := builder.NewArray()
							array.Release()
							builder.Release()
						case "strings":
							builder := NewBinaryBuilder(allocator, arrow.BinaryTypes.Binary)
							builder.AppendStringValues(stringValues, valid)
							array := builder.NewArray()
							array.Release()
							builder.Release()
						case "fixed32":
							builder := NewFixedSizeBinaryBuilder(allocator, &arrow.FixedSizeBinaryType{ByteWidth: 32})
							builder.AppendValues(fixedValues, valid)
							array := builder.NewArray()
							array.Release()
							builder.Release()
						}
					}
				})
			}
		}
	}
}
