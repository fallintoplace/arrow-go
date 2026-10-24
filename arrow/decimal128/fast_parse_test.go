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

package decimal128_test

import (
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/decimal128"
)

// Check exact values, offsets, signs, scientific notation and the slow-path
// rounding cases against independently constructed integer coefficients.
func TestFastDecimalStringParse(t *testing.T) {
	cases := []struct {
		value string
		prec, scale int32
		want string
	}{
		{"0", 20, 0, "0"},
		{"-0.000", 20, 3, "0"},
		{"+00123.4500", 20, 4, "1234500"},
		{"-.125", 20, 3, "-125"},
		{"12.", 20, 0, "12"},
		{"999999999999999999", 20, 0, "999999999999999999"},
		{"12.3", 20, 2, "1230"},
		{"1e2", 20, 0, "100"},
		{"1.237", 20, 2, "124"},
		{"-1.237", 20, 2, "-124"},
		{"12345678901234567890123456789012345678", 38, 0, "12345678901234567890123456789012345678",
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := decimal128.FromString(tc.value, tc.prec, tc.scale)
			if err != nil {
				t.Fatal(err)
			}
			want, ok := new(big.Int).SetString(tc.want, 10)
			if !ok || got.BigInt().Cmp(want) != 0 {
				t.Fatalf("FromString(%q, %d, %d) = %s, want %s", tc.value, tc.prec, tc.scale, got.BigInt(), tc.want)
			}
		})
	}
	for _, input := range []string{"", ".", "-", "+", "1.2.3", "garbage"} {
		if _, err := decimal128.FromString(input, 20, 2); err == nil {
			t.Errorf("expected error for %q", input)
		}
	}
	if _, err := decimal128.FromString("12.34", 3, 2); err == nil {
		t.Fatal("expected precision overflow")
	}
}

// Deterministic input combinations cover sign, fractional padding, and
// valid exact values without taking the big.Float path.
func TestFastDecimalStringParseRandomExact(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for scale := int32(0); scale <= 9; scale++ {
		for i := 0; i < 500; i++ {
			magnitude := uint64(rng.Int63n(1_000_000_000))
			fractional := rng.Intn(int(scale) + 1)
			value := strconv.FormatUint(magnitude, 10)
			if len(value) <= fractional {
				value = strings.Repeat("0", fractional-len(value)+1) + value
			}
			if fractional > 0 {
				cut := len(value) - fractional
				value = value[:cut] + "." + value[cut:]
			}
			want := new(big.Int).SetUint64(magnitude)
			power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)-int64(fractional)), nil)
			want.Mul(want, power)
			if i%3 == 0 {
				value = "-" + value
				want.Neg(want)
			}
			got, err := decimal128.FromString(value, decimal128.MaxPrecision, scale)
			if err != nil || got.BigInt().Cmp(want) != 0 {
				t.Fatalf("value=%q scale=%d: got=%s err=%v, want=%s", value, scale, got.BigInt(), err, want)
			}
		}
	}
}

var fastDecimalParseSink decimal128.Num

func BenchmarkFastDecimalStringParse(b *testing.B) {
	cases := []struct {
		name, value string
		prec, scale int32
	}{
		{"integer", "123456789", 20, 0},
		{"fractional", "1234567.89", 20, 2},
		{"negative", "-98765.4321", 20, 4},
		{"leading_zeros", "+000123.4500", 20, 4},
		{"fallback_rounding", "1234567.899", 20, 2},
		{"fallback_exponent", "1.234567e3", 20, 2},
		{"large", "12345678901234567890123456789012345678", 38, 0,
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				value, err := decimal128.FromString(tc.value, tc.prec, tc.scale)
				if err != nil {
					b.Fatal(err)
				}
				fastDecimalParseSink = value
			}
		})
	}
}
