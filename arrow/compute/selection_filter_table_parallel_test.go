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

//go:build go1.18

package compute_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/compute"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/require"
)

func TestFilterTableNonPositiveParallelism(t *testing.T) {
	mem := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer mem.AssertSize(t, 0)

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int32},
		{Name: "label", Type: arrow.BinaryTypes.String},
	}, nil)
	input, err := array.TableFromJSON(mem, schema, []string{
		`[{"id": 1, "label": "one"}, {"id": 2, "label": "two"}]`,
		`[{"id": 3, "label": "three"}, {"id": 4, "label": "four"}]`,
	})
	require.NoError(t, err)
	defer input.Release()

	filter, err := array.ChunkedFromJSON(mem, arrow.FixedWidthTypes.Boolean, []string{
		`[false, true]`,
		`[true, null]`,
	})
	require.NoError(t, err)
	defer filter.Release()

	expected, err := array.TableFromJSON(mem, schema, []string{
		`[{"id": 2, "label": "two"}]`,
		`[{"id": 3, "label": "three"}]`,
	})
	require.NoError(t, err)
	defer expected.Release()

	for _, numParallel := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprintf("numParallel=%d", numParallel), func(t *testing.T) {
			execCtx := compute.DefaultExecCtx()
			execCtx.NumParallel = numParallel
			ctx := compute.SetExecCtx(compute.WithAllocator(context.Background(), mem), execCtx)

			actual, err := compute.FilterTable(ctx, input, &compute.ChunkedDatum{Value: filter}, compute.DefaultFilterOptions())
			require.NoError(t, err)
			defer actual.Release()
			require.Truef(t, array.TableEqual(expected, actual), "expected: %s\ngot: %s", expected, actual)
		})
	}
}
