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

package decimalparse

import "testing"

func TestExactScaledUint64(t *testing.T) {
	tests := []struct {
		value string
		scale int32
		want uint64
		negative, ok bool
	}{
		{"123.45", 2, 12345, false, true},
		{"+00012.3", 2, 1230, false, true},
		{"-.25", 2, 25, true, true},
		{"-0", 0, 0, true, true},
		{".125", 3, 125, false, true},
		{"12.", 0, 12, false, true},
		{"999999999999999999", 0, 999999999999999999, false, true},
		{"9999999999999999999", 0, 0, false, false},
		{"1.23", 1, 0, false, false},
		{"1e3", 0, 0, false, false},
		{"1.2.3", 2, 0, false, false},
		{"", 1, 0, false, false},
		{".", 2, 0, false, false},
		{"--1", 2, 0, false, false},
		{"12", -1, 0, false, false},
		{"12", 77, 0, false, false},
	}
	for _, tt := range tests {
		got, neg, ok := ExactScaledUint64(tt.value, tt.scale)
		if got != tt.want || neg != tt.negative || ok != tt.ok {
			t.Errorf("%q scale=%d: got (%d,%v,%v), want (%d,%v,%v)",
				tt.value, tt.scale, got, neg, ok, tt.want, tt.negative, tt.ok)
		}
	}
}
