package priceshandler

import (
	"errors"
	"testing"
)

func validDef() PriceDef {
	return PriceDef{PriceDefName: " deepseek-pricing ", PriceType: "llm", Items: []Item{
		{ItemName: " input-cache-hit-peak ", ItemType: "input", Cache: "hit", TimeSpan: "peak",
			Unit: "million-tokens", Currency: " cn ", Value: " 0.04 "},
		{ItemName: "output-off", ItemType: "output", TimeSpan: "off-peak",
			Unit: "million-tokens", Currency: "CN", Value: "4.00"},
	}}
}

func TestValidatePriceDef(t *testing.T) {
	p := validDef()
	if err := validate(&p); err != nil {
		t.Fatal(err)
	}
	if p.PriceDefName != "deepseek-pricing" || p.Items[0].ItemName != "input-cache-hit-peak" ||
		p.Items[0].Currency != "CN" || p.Items[0].Value != "0.04" {
		t.Fatalf("fields were not normalized: %+v", p)
	}
}

func TestValidatePriceDefRejects(t *testing.T) {
	cases := map[string]func(*PriceDef){
		"empty name":     func(p *PriceDef) { p.PriceDefName = " " },
		"bad type":       func(p *PriceDef) { p.PriceType = "other" },
		"no items":       func(p *PriceDef) { p.Items = nil },
		"duplicate item": func(p *PriceDef) { p.Items[1].ItemName = "input-cache-hit-peak" },
		"bad item type":  func(p *PriceDef) { p.Items[0].ItemType = "both" },
		"bad cache":      func(p *PriceDef) { p.Items[0].Cache = "partial" },
		"bad time span":  func(p *PriceDef) { p.Items[0].TimeSpan = "night" },
		"no currency":    func(p *PriceDef) { p.Items[0].Currency = "" },
		"negative value": func(p *PriceDef) { p.Items[0].Value = "-1" },
		"not a number":   func(p *PriceDef) { p.Items[0].Value = "abc" },
		"exponent":       func(p *PriceDef) { p.Items[0].Value = "1e3" },
		"NaN":            func(p *PriceDef) { p.Items[0].Value = "NaN" },
	}
	for name, mutate := range cases {
		p := validDef()
		mutate(&p)
		if err := validate(&p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}
