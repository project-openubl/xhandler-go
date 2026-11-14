package xbuilder

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Date is a calendar date, sent as "YYYY-MM-DD".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate returns the date year-month-day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{Year: year, Month: month, Day: day}
}

// DateOf returns the date of t in its location.
func DateOf(t time.Time) Date {
	return NewDate(t.Date())
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// MarshalJSON implements json.Marshaler.
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", s, err)
	}
	*d = DateOf(t)
	return nil
}

// Time is a time of day, sent as "HH:MM:SS".
type Time struct {
	Hour, Minute, Second int
}

// NewTime returns the time of day hour:minute:second.
func NewTime(hour, minute, second int) Time {
	return Time{Hour: hour, Minute: minute, Second: second}
}

func (t Time) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
}

// MarshalJSON implements json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler. Fractions of a second are dropped.
func (t *Time) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.TimeOnly, s) // also accepts "10:30:00.5"
	if err != nil {
		return fmt.Errorf("invalid time %q: %w", s, err)
	}
	*t = NewTime(parsed.Clock())
	return nil
}

// Decimal is a decimal number that keeps its scale: "118.00" is sent as
// "118.00", not "118", because xbuilder renders some amounts as given.
// It embeds a shopspring decimal.Decimal for the arithmetic.
type Decimal struct {
	decimal.Decimal
}

// NewDecimal parses a decimal number such as "118.00".
func NewDecimal(value string) (Decimal, error) {
	d, err := decimal.NewFromString(value)
	return Decimal{d}, err
}

// MustDecimal is like NewDecimal but panics on an invalid number. It is meant
// for constants.
func MustDecimal(value string) Decimal {
	d, err := NewDecimal(value)
	if err != nil {
		panic(err)
	}
	return d
}

// DecimalFromInt returns the integer value as a Decimal.
func DecimalFromInt(value int64) Decimal {
	return Decimal{decimal.NewFromInt(value)}
}

// String returns the number with its scale, e.g. "118.00".
func (d Decimal) String() string {
	if exp := d.Exponent(); exp < 0 {
		return d.StringFixed(-exp)
	}
	return d.Decimal.String()
}

// MarshalJSON implements json.Marshaler, sending the number as a string.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// MarshalText implements encoding.TextMarshaler, keeping the scale too.
func (d Decimal) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// Ptr returns a pointer to v, for the optional fields of the models:
// Moneda: xbuilder.Ptr("USD").
func Ptr[T any](v T) *T {
	return &v
}
