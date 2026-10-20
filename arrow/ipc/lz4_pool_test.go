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

package ipc

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/internal/flatbuf"
	"github.com/stretchr/testify/require"
)

func TestLZ4DecompressorReuseAcrossBatches(t *testing.T) {
	const codec = flatbuf.CompressionTypeLZ4_FRAME
	for i, n := range []int{1, 100, 4096, 64 << 10, 17, 300_000} {
		want := compressibleBytes(n, int64(i))
		compressed := compressBuffer(t, codec, want)
		decoder := getDecompressor(codec)

		if i == 3 {
			// A failed decode must not poison a decoder returned to the pool.
			require.Error(t, decoder.Decompress(make([]byte, n), compressed[:len(compressed)/2]))
		}

		got := make([]byte, n)
		require.NoError(t, decoder.Decompress(got, compressed))
		require.Equal(t, want, got)
		decoder.Close()
	}
}

// BenchmarkLZ4DecompressorReuse measures one decoder handling multiple IPC
// body buffers, including decoder checkout and return between record batches.
func BenchmarkLZ4DecompressorReuse(b *testing.B) {
	const codec = flatbuf.CompressionTypeLZ4_FRAME
	for _, size := range []int{1024, 64 << 10} {
		payload := compressibleBytes(size, 42)
		var encoded bytes.Buffer
		compressor := getCompressor(codec, 0)
		compressor.Reset(&encoded)
		if _, err := compressor.Write(payload); err != nil {
			b.Fatal(err)
		}
		if err := compressor.Close(); err != nil {
			b.Fatal(err)
		}

		for _, buffers := range []int{1, 16, 64} {
			b.Run(fmt.Sprintf("%dB/%dbuffers", size, buffers), func(b *testing.B) {
				dst := make([]byte, size)
				b.ReportAllocs()
				b.SetBytes(int64(size * buffers))
				for b.Loop() {
					decoder := getDecompressor(codec)
					for range buffers {
						if err := decoder.Decompress(dst, encoded.Bytes()); err != nil {
							decoder.Close()
							b.Fatal(err)
						}
					}
					decoder.Close()
				}
				b.StopTimer()
				if !bytes.Equal(dst, payload) {
					b.Fatal("decompressed contents differ")
				}
			})
		}
	}
}
