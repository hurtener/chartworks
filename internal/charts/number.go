package charts

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// decimal deliberately avoids Rat.SetString's fractions, bases and unbounded
// exponents. Input and exponent expansion are bounded before allocating big ints.
func decimal(text string) (*big.Rat, int, error) {
	if len(text) == 0 || len(text) > 4096 {
		return nil, 0, ErrInvalid
	}
	s := text
	negative := false
	if s[0] == '-' || s[0] == '+' {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return nil, 0, ErrInvalid
	}
	exponent := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e := s[i+1:]
		if len(e) == 0 || len(e) > 5 {
			return nil, 0, ErrInvalid
		}
		var err error
		exponent, err = strconv.Atoi(e)
		if err != nil || exponent < -4096 || exponent > 4096 {
			return nil, 0, ErrInvalid
		}
		s = s[:i]
	}
	digits := make([]byte, 0, len(s))
	scale, dot := 0, false
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			if dot || i == 0 || i == len(s)-1 {
				return nil, 0, ErrInvalid
			}
			dot = true
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return nil, 0, ErrInvalid
		}
		digits = append(digits, s[i])
		if dot {
			scale++
		}
	}
	if len(digits) == 0 {
		return nil, 0, ErrInvalid
	}
	n, ok := new(big.Int).SetString(string(digits), 10)
	if !ok {
		return nil, 0, ErrInvalid
	}
	if negative {
		n.Neg(n)
	}
	scale -= exponent
	power := scale
	if power < 0 {
		power = -power
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil)
	r := new(big.Rat)
	if scale < 0 {
		r.SetInt(n.Mul(n, factor))
		return r, 0, nil
	}
	r.SetFrac(n, factor)
	return r, scale, nil
}

func numeric(typ string) bool { return typ == "integer" || typ == "decimal" || typ == "number" }

func numberValue(c Cell) (Value, error) {
	if c.Null {
		return Value{Null: true}, nil
	}
	r, _, err := decimal(c.Value)
	if err != nil {
		return Value{}, err
	}
	f, exact := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) || f == 0 && r.Sign() != 0 {
		return Value{}, ErrUnsuitable
	}
	return Value{Exact: c.Value, Coordinate: &f, Approximate: !exact}, nil
}
