// src/lib/types.ts

/*
export type Field =
  | 'record_uuid'
  | 'doc_name'
  | 'doc_num'
  | 'md5'
  | 'file_url'
  | 'status'
  | 'data_proc_status';
  */

export type FieldType = string
export type Operator =
  | '>'
  | '>='
  | '='
  | '<='
  | '<'
  | '<>'
  | 'contain'
  | 'prefix';

// Optional: if you want per-condition logic, but for simplicity, we'll use global logic first
export type LogicalOperator = 'AND' | 'OR';

export type AtomicCondition = {
  field_name:   FieldType;
  operator:     Operator;
  value_1:      string;
  value_2:      string;
}

export type ComplexCondition = {
  operator:     LogicalOperator;
  conditions:   (AtomicCondition | ComplexCondition)[];
};

export type QueryCondition = AtomicCondition | ComplexCondition;

// Data types for table ProcessStatus
export interface SeriesEntry {
  name: string;
  data: number[];
}

export interface ChartData {
  categories: string[];
  series: SeriesEntry[];
}

export interface ProcessStatusResponse {
  status:         boolean;
  error_msg:      string;
  num_records:    number;
  loc:            string;
  map_data:       {[key: string]: ChartData};
}

export type JimoRequest = {
  request_type:   string,
  opr:            string,
  resource:       string,
  conditions:     QueryCondition
}