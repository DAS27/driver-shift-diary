// Package diary implements exact money arithmetic and driver-local daily reports.
package diary

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Money stores minor units (tiyn). JSON uses decimal tenge, never floating point.
type Money int64

const MaxMoney Money = 1_000_000_000_00

var decimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,2})?$`)

func (m *Money) UnmarshalJSON(b []byte) error {
	s := string(bytes.TrimSpace(b))
	if !decimal.MatchString(s) {
		return errors.New("money must be a number with at most 2 decimal places")
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > int64(MaxMoney/100) {
		return errors.New("money exceeds allowed limit")
	}
	var fraction int64
	if len(parts) == 2 {
		f := parts[1]
		if len(f) == 1 {
			f += "0"
		}
		fraction, _ = strconv.ParseInt(f, 10, 64)
	}
	value := Money(whole*100 + fraction)
	if value > MaxMoney {
		return errors.New("money exceeds allowed limit")
	}
	if negative {
		value = -value
	}
	*m = value
	return nil
}
func (m Money) MarshalJSON() ([]byte, error) {
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	return []byte(fmt.Sprintf("%s%d.%02d", sign, m/100, m%100)), nil
}

type Trip struct {
	ID         string    `json:"id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Amount     Money     `json:"amount"`
	Payment    string    `json:"payment"`
	Commission Money     `json:"commission"`
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

func (t Trip) Validate() error {
	if t.ID != "" && !validID.MatchString(t.ID) {
		return errors.New("id must contain 1–100 letters, digits, underscores or hyphens")
	}
	if t.Start.IsZero() || t.End.IsZero() || t.Start.Year() < 2000 || t.End.Year() > 2100 {
		return errors.New("start and end must be RFC3339 dates between 2000 and 2100")
	}
	if !t.End.After(t.Start) {
		return errors.New("end must be later than start")
	}
	if t.Amount <= 0 || t.Amount > MaxMoney {
		return errors.New("amount must be greater than zero and at most 1000000000")
	}
	if t.Commission < 0 || t.Commission > t.Amount {
		return errors.New("commission must be between zero and amount")
	}
	if t.Payment != "cash" && t.Payment != "card" {
		return errors.New("payment must be cash or card")
	}
	return nil
}

// Fingerprint normalizes timestamps: offset spelling and caller IDs do not affect identity.
func (t Trip) Fingerprint() string {
	return fmt.Sprintf("%s|%s|%d|%s|%d", t.Start.UTC().Format(time.RFC3339Nano), t.End.UTC().Format(time.RFC3339Nano), t.Amount, t.Payment, t.Commission)
}

type Totals struct {
	Count      int   `json:"count"`
	Revenue    Money `json:"revenue"`
	Commission Money `json:"commission"`
	Net        Money `json:"net"`
}

func (s *Totals) add(t Trip) {
	s.Count++
	s.Revenue += t.Amount
	s.Commission += t.Commission
	s.Net += t.Amount - t.Commission
}

type Summary struct {
	Totals
	Cash Totals `json:"cash"`
	Card Totals `json:"card"`
}

func Summarize(trips []Trip) Summary {
	var s Summary
	for _, t := range trips {
		s.Totals.add(t)
		if t.Payment == "cash" {
			s.Cash.add(t)
		} else {
			s.Card.add(t)
		}
	}
	return s
}

// DecodeTrip rejects missing required monetary fields, unknown fields and nulls.
func DecodeTrip(data []byte) (Trip, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Trip{}, err
	}
	for _, key := range []string{"start", "end", "amount", "payment", "commission"} {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Trip{}, fmt.Errorf("%s is required", key)
		}
	}
	var t Trip
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&t); err != nil {
		return Trip{}, err
	}
	return t, t.Validate()
}
