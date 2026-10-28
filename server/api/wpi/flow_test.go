package wpi

import "testing"

func TestFlowLayouterConfig(t *testing.T) {
	for _, config := range []string{
		`{}`, `{"column_gap":"0px","row_gap":"1.5rem"}`,
		`{"horizontal_alignment":"start","vertical_alignment":"top","rows_alignment":"top"}`,
		`{"horizontal_alignment":"center","vertical_alignment":"center","rows_alignment":"center"}`,
		`{"horizontal_alignment":"end","vertical_alignment":"bottom","rows_alignment":"bottom"}`,
		`{"horizontal_alignment":"space-between","vertical_alignment":"stretch","rows_alignment":"space-between"}`,
	} {
		t.Run(config, func(t *testing.T) {
			page := decode(t, pageWith(``))
			page["layouter_config"] = decode(t, config)
			expectIssue(t, ValidateDescriptor(KindPage, "p", page), "")
		})
	}
	for _, tc := range []struct{ config, path string }{
		{`null`, "/layouter_config"}, {`[]`, "/layouter_config"}, {`"center"`, "/layouter_config"},
		{`{"gap":"12px"}`, `/layouter_config/gap unknown attribute "gap"`},
		{`{"column_gap":"-1px"}`, "/layouter_config/column_gap"},
		{`{"row_gap":"10%"}`, "/layouter_config/row_gap"},
		{`{"row_gap":"1em"}`, "/layouter_config/row_gap"},
		{`{"row_gap":0}`, "/layouter_config/row_gap"},
		{`{"column_gap":"0"}`, "/layouter_config/column_gap"},
		{`{"column_gap":"1px; color:red"}`, "/layouter_config/column_gap"},
		{`{"horizontal_alignment":"left"}`, "/layouter_config/horizontal_alignment"},
		{`{"vertical_alignment":"start"}`, "/layouter_config/vertical_alignment"},
		{`{"rows_alignment":"stretch"}`, "/layouter_config/rows_alignment"},
	} {
		t.Run(tc.config, func(t *testing.T) {
			page := decode(t, `{"descriptor_id":"p","schema_version":"0.2","layouter":"flow-layouter","elements":[],"layouter_config":`+tc.config+`}`)
			expectIssue(t, ValidateDescriptor(KindPage, "p", page), tc.path)
		})
	}
}
