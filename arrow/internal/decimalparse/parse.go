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

// ExactScaledUint64 parses a plain decimal as an integer coefficient scaled
// by 10^scale without allocating. Inputs requiring rounding, exceeding 18
// scaled digits, or using exponent notation must use the caller's slow path.
// An unsuccessful parse never changes the caller's error semantics.
func ExactScaledUint64(value string, scale int32) (magnitude uint64, negative bool, ok bool) {
	if scale < 0 || scale > 76 || len(value) == 0 {
		return 0, false, false
	}

	start := 0
	switch value[0] {
	case '-':
		negative = true
		start = 1
	case '+':
		start = 1
	}

	var digits, fractional int
	var hasPoint bool
	for i := start; i < len(value); i++ {
		c := value[i]
		if c == '.' {
			if hasPoint {
				return 0, false, false
			}
			hasPoint = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false, false
		}
		digits++
		if digits > 18 {
			return 0, false, false
		}
		magnitude = magnitude*10 + uint64(c-'0')
		if hasPoint {
			fractional++
		}
	}

	padding := int(scale) - fractional
	if digits == 0 || padding < 0 || digits+padding > 18 {
		return 0, false, false
	}
	for range padding {
		magnitude *= 10
	}
	return magnitude, negative, true
}
