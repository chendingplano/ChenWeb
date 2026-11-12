package docprocessing

import "testing"

func TestRawLinesToCompactJSON(t *testing.T) {
	got := rawLinesToCompactJSON([]Line{
		{LineNo: 1, PageNo: 1, LineType: "paragraph", Content: "A & <b>"},
		{LineNo: 2, PageNo: 1, LineType: "image", Content: "x"},
		{LineNo: 3, PageNo: 2, LineType: "heading-1", Content: "H"},
	})
	want := `[["flag","line_number","page_number","line_type","content"],["n",1,1,"paragraph","A & <b>"],["n",3,2,"heading-1","H"]]`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}
