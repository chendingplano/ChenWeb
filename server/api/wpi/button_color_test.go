package wpi

import "testing"

func TestButtonBackgroundColor(t *testing.T) {
	for _, tc := range []struct{ attrs, want string }{
		{`"background_color":"#fff"`, ""},
		{`"background_color":{"light":"#fff","dark":"#123456"}`, ""},
		{`"background_color":"auto"`, "/background_color"},
		{`"background_color":{"light":"#fff"}`, "/background_color/dark"},
		{`"background_color":42`, "/background_color"},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			v := &validator{kind: KindElement, id: "button"}
			v.element("button", decode(t, `{`+tc.attrs+`}`), "")
			expectIssue(t, v.issues, tc.want)
		})
	}
}
