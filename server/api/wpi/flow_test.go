package wpi

import "testing"

// TestFlowLayouterConfig covers the parameters of a panel's flow-layouter copy,
// which replace the copy's config whole (OpenSpec wpi-pages-own-panes, design D2).
func TestFlowLayouterConfig(t *testing.T) {
	page := func(t *testing.T, config string) map[string]any {
		p := withCopies(t, testClasses(), "p", decode(t, pageWith(``)))
		layouter := p["panels"].(map[string]any)["main"].(map[string]any)["elem_layouter"].(map[string]any)
		layouter["config"] = decode(t, `{"c":`+config+`}`)["c"]
		return p
	}
	for _, config := range []string{
		`{}`, `{"left_margin":"0px","right_margin":"1rem","top_margin":"20px","bottom_margin":"30px"}`, `{"column_gap":"0px","row_gap":"1.5rem"}`,
		`{"horizontal_alignment":"start","vertical_alignment":"top","rows_alignment":"top"}`,
		`{"horizontal_alignment":"center","vertical_alignment":"center","rows_alignment":"center"}`,
		`{"horizontal_alignment":"end","vertical_alignment":"bottom","rows_alignment":"bottom"}`,
		`{"horizontal_alignment":"space-between","vertical_alignment":"stretch","rows_alignment":"space-between"}`,
		`{"min_row_height":"30px","max_row_height":"200px","min_elem_width":"20px","max_elem_width":"300px","max_num_elems":0,"row_separator":"thin_solid_line","elem_separator":"bold_dash_line"}`,
	} {
		t.Run(config, func(t *testing.T) {
			expectIssue(t, ValidateDescriptor(KindPage, "p", page(t, config)), "")
		})
	}
	const path = "/panels/main/elem_layouter/config"
	for _, tc := range []struct{ config, path string }{
		{`null`, path}, {`[]`, path}, {`"center"`, path},
		{`{"gap":"12px"}`, path + `/gap unknown attribute "gap"`},
		{`{"left_margin":"-1px"}`, path + "/left_margin"},
		{`{"column_gap":"-1px"}`, path + "/column_gap"},
		{`{"row_gap":"10%"}`, path + "/row_gap"},
		{`{"row_gap":"1em"}`, path + "/row_gap"},
		{`{"row_gap":0}`, path + "/row_gap"},
		{`{"column_gap":"0"}`, path + "/column_gap"},
		{`{"column_gap":"1px; color:red"}`, path + "/column_gap"},
		{`{"horizontal_alignment":"left"}`, path + "/horizontal_alignment"},
		{`{"vertical_alignment":"start"}`, path + "/vertical_alignment"},
		{`{"rows_alignment":"stretch"}`, path + "/rows_alignment"},
		{`{"max_num_elems":-1}`, path + "/max_num_elems"},
		{`{"max_num_elems":1.5}`, path + "/max_num_elems"},
		{`{"min_row_height":"-1px"}`, path + "/min_row_height"},
		{`{"elem_separator":"dotted"}`, path + "/elem_separator"},
	} {
		t.Run(tc.config, func(t *testing.T) {
			expectIssue(t, ValidateDescriptor(KindPage, "p", page(t, tc.config)), tc.path)
		})
	}
}
