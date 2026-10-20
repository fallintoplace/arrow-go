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
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestRecordReaderReleasesFinalBatchAtEOF(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.NewGoAllocator())
	defer mem.AssertSize(t, 0)

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "value", Type: arrow.PrimitiveTypes.Int32},
	}, nil)

	builder := array.NewInt32Builder(mem)
	builder.AppendValues([]int32{1, 2, 3}, nil)
	values := builder.NewInt32Array()
	builder.Release()

	rec := array.NewRecordBatch(schema, []arrow.Array{values}, 3)
	values.Release()

	reader, err := array.NewRecordReader(schema, []arrow.RecordBatch{rec})
	rec.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Release()

	if reader.RecordBatch() != nil {
		t.Fatal("expected no current batch before the first Next")
	}
	if !reader.Next() || reader.RecordBatch() == nil {
		t.Fatal("expected a current batch after advancing")
	}
	if reader.Next() {
		t.Fatal("expected EOF after the only batch")
	}
	if reader.RecordBatch() != nil {
		t.Fatal("expected current batch to be cleared at EOF")
	}
	if allocated := mem.CurrentAlloc(); allocated != 0 {
		t.Fatalf("last batch is still allocated after EOF: %d bytes", allocated)
	}
	if reader.Next() {
		t.Fatal("expected subsequent Next to remain at EOF")
	}
}
