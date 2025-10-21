// src/lib/types.ts

export type Field =
  | 'record_uuid'
  | 'doc_name'
  | 'doc_num'
  | 'md5'
  | 'file_url'
  | 'status'
  | 'data_proc_status';

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

export interface Condition {
  field: Field;
  operator: Operator;
  value: string;
  logic_opr: LogicalOperator;
}
