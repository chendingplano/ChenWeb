// About This File:
// This file contains custom cell renderers. They are used as implementation examples.
<script>
	import { string } from "zod/v4";

</script>

// Source: https://www.ag-grid.com/react-data-grid/deep-dive/
// Custom Cell Renderer (Display logos based on cell value)
const CompanyLogoRenderer = (params: CustomCellRendererProps) => (
  <span
    style={{
      display: "flex",
      height: "100%",
      width: "100%",
      alignItems: "center",
    }}
  >
    {params.value && (
      <img
        alt={`${params.value} Flag`}
        src={`https://www.ag-grid.com/example-assets/space-company-logos/${params.value.toLowerCase()}.png`}
        style={{
          display: "block",
          width: "25px",
          height: "auto",
          maxHeight: "50%",
          marginRight: "12px",
          filter: "brightness(1.1)",
        }}
      />
    )}
    <p
      style={{
        textOverflow: "ellipsis",
        overflow: "hidden",
        whiteSpace: "nowrap",
      }}
    >
      {params.value}
    </p>
  </span>
);

// Custom Cell Renderer (Display tick / cross in 'Successful' column)
// The field should be a boolean. If its value is true, it shows 'tick-in-circle', 
// otherwise 'cross-in-circle'
const MissionResultRenderer = (params: CustomCellRendererProps) => (
  <span
    style={{
      display: "flex",
      justifyContent: "center",
      height: "100%",
      alignItems: "center",
    }}
  >
    {
      <img
        alt={`${params.value}`}
        src={`https://www.ag-grid.com/example-assets/icons/${params.value ? "tick-in-circle" : "cross-in-circle"}.png`}
        style={{ width: "auto", height: "auto" }}
      />
    }
  </span>
);

// Format Date Cells
// The display format is like: "Monday, Jan 1, 2020"
// The field value can be string
const dateFormatter = (params: ValueFormatterParams): string => {
  return new Date(params.value).toLocaleDateString("en-us", {
    weekday: "long",
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};