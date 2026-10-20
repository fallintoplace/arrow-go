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

package csv_test

import (
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/csv"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/require"
)

func TestCSVListValuesPreserveBraces(t *testing.T) {
	tests := []struct {
		name      string
		typ       arrow.DataType
		valueJSON string
	}{
		{"list string", arrow.ListOf(arrow.BinaryTypes.String), "[[\"{value}\"]]"},
		{"large list string", arrow.LargeListOf(arrow.BinaryTypes.String), "[[\"{value}\"]]"},
		{"fixed-size list string", arrow.FixedSizeListOf(1, arrow.BinaryTypes.String), "[[\"{value}\"]]"},
		{"nested empty list", arrow.ListOf(arrow.ListOf(arrow.PrimitiveTypes.Int64)), "[[[]]]"},
		{"fixed-size nested empty list", arrow.FixedSizeListOf(1, arrow.ListOf(arrow.PrimitiveTypes.Int64)), "[[[]]]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expected, _, err := array.FromJSON(memory.DefaultAllocator, tc.typ, strings.NewReader(tc.valueJSON))
			require.NoError(t, err)
			defer expected.Release()

			schema := arrow.NewSchema([]arrow.Field{{Name: "values", Type: tc.typ}}, nil)
			rec := array.NewRecordBatch(schema, []arrow.Array{expected}, 1)
			defer rec.Release()

			var buf strings.Builder
			writer := csv.NewWriter(&buf, schema)
			require.NoError(t, writer.Write(rec))

			reader := csv.NewReader(strings.NewReader(buf.String()), schema)
			defer reader.Release()
			require.True(t, reader.Next())
			require.NoError(t, reader.Err())
			require.True(t, array.Equal(expected, reader.RecordBatch().Column(0)),
				"CSV round-trip changed %s: wrote %q, got %s", tc.typ, buf.String(), reader.RecordBatch().Column(0))
			require.False(t, reader.Next())
			require.NoError(t, reader.Err())
		})
	}
}
