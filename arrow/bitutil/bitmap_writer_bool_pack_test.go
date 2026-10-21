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

func TestBitmapWriterAppendBoolsBulkParity(t *testing.T) {
	lengths := []int{0, 1, 2, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129, 257, 1024}
	offsets := []int{0, 1, 3, 7, 8, 9, 15, 16, 23}
	chunkSizes := []int{1, 2, 7, 8, 9, 16, 33, 64, 2048}

	for _, length := range lengths {
		for _, offset := range offsets {
			for _, fill := range []byte{0x00, 0x55, 0xff} {
				values := make([]bool, length)
				for i := range values {
					values[i] = (i*13+offset)%7 < 3
				}
				initial := bytes.Repeat([]byte{fill}, (offset+length+7)/8+2)
				expected := bytes.Clone(initial)
				for i, v := range values {
					bitutil.SetBitTo(expected, offset+i, v)
				}

				for _, chunk := range chunkSizes {
					t.Run(fmt.Sprintf("len=%d/offset=%d/fill=%02x/chunk=%d", length, offset, fill, chunk), func(t *testing.T) {
						actual := bytes.Clone(initial)
						writer := bitutil.NewBitmapWriter(actual, offset, length)
						if writer.AppendBools(nil) != 0 {
							t.Fatal("nil input appended nonzero values")
						}
						for i := 0; i < length; i += chunk {
							end := min(i+chunk, length)
							if got := writer.AppendBools(values[i:end]); got != end-i {
								t.Fatalf("appended %d values, expected %d", got, end-i)
							}
						}
						if writer.AppendBools([]bool{true}) != 0 {
							t.Fatal("appended past the writer length")
						}
						writer.Finish()
						if !bytes.Equal(actual, expected) {
							t.Fatalf("wrong bitmap: got %x, want %x", actual, expected)
						}
					})
				}
			}
		}
	}
}

func TestBitmapWriterAppendBoolsWithScalarAndBitmap(t *testing.T) {
	for _, offset := range []int{0, 1, 3, 7, 8, 11} {
		const length = 139
		values := make([]bool, length)
		for i := range values {
			values[i] = i%3 != 0
		}
		initial := bytes.Repeat([]byte{0xa5}, (offset+length+7)/8+2)
		expected := bytes.Clone(initial)
		for i, v := range values {
			bitutil.SetBitTo(expected, offset+i, v)
		}

		actual := bytes.Clone(initial)
		writer := bitutil.NewBitmapWriter(actual, offset, length)
		for i := 0; i < length; {
			if i%13 == 0 {
				if values[i] {
					writer.Set()
				} else {
					writer.Clear()
				}
				writer.Next()
				i++
			} else {
				end := min(i+9, length)
				if got := writer.AppendBools(values[i:end]); got != end-i {
					t.Fatalf("offset %d: appended %d values, expected %d", offset, got, end-i)
				}
				i = end
			}
		}
		writer.Finish()
		if !bytes.Equal(actual, expected) {
			t.Fatalf("offset %d: got %x, want %x", offset, actual, expected)
		}
	}

	bitmap := []byte{0x00, 0x00, 0xff}
	writer := bitutil.NewBitmapWriter(bitmap, 0, 16)
	writer.AppendBools([]bool{true, true, true, true, true, true, true, true})
	writer.AppendBitmap([]byte{0xa5}, 0, 8)
	writer.Finish()
	if !bytes.Equal(bitmap, []byte{0xff, 0xa5, 0xff}) {
		t.Fatalf("AppendBitmap after an aligned AppendBools: %x", bitmap)
	}
}

func TestBitmapWriterAppendBoolsReset(t *testing.T) {
	initial := bytes.Repeat([]byte{0x55}, 20)
	expected := bytes.Clone(initial)
	actual := bytes.Clone(initial)
	writer := bitutil.NewBitmapWriter(actual, 0, 64)

	first := []bool{true, false, true, true, false, false, true, false}
	writer.AppendBools(first)
	for i, v := range first {
		bitutil.SetBitTo(expected, i, v)
	}

	writer.Reset(3, 65)
	values := make([]bool, 65)
	for i := range values {
		values[i] = i%3 == 0
		bitutil.SetBitTo(expected, 3+i, values[i])
	}
	writer.AppendBools(values)
	writer.Finish()
	if !bytes.Equal(actual, expected) {
		t.Fatalf("bitmap after Reset: got %x, want %x", actual, expected)
	}
}
