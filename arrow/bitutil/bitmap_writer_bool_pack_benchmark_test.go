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
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/bitutil"
)

func BenchmarkBitmapWriterAppendBools(b *testing.B) {
	for _, length := range []int{64, 8192, 65536} {
		for _, offset := range []int{0, 3} {
			for _, pattern := range []string{"all_true", "two_in_three"} {
				b.Run(fmt.Sprintf("len=%d/offset=%d/%s", length, offset, pattern), func(b *testing.B) {
					values := make([]bool, length)
					for i := range values {
						values[i] = pattern == "all_true" || i%3 != 0
					}
					dest := make([]byte, (offset+length+7)/8)
					b.SetBytes(int64(length))
					b.ReportAllocs()
					b.ResetTimer()

					var n int
					for i := 0; i < b.N; i++ {
						writer := bitutil.NewBitmapWriter(dest, offset, length)
						n = writer.AppendBools(values)
						writer.Finish()
					}
					if n != length {
						b.Fatalf("appended %d values, expected %d", n, length)
					}
				})
			}
		}
	}
}
