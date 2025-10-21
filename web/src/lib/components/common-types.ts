// Row Data Interface
export interface IProcessTableRow {
  		record_uuid: string;
  		md5: string;
  		doc_name: string;
  		doc_num: string;
  		file_url: string;
  		status: string;
  		data_proc_status: string;
		data_proc_at:string;
		chunk_proc_status:string;
		chunk_proc_at:string;
		embedding_proc_status:string;
		embedding_proc_at:string;
		es_proc_status:string;
		es_proc_at:string;
		parser_source_tname:string;
		parser_content_proc_tname:string;
		chunk_tablename:string;
		error_msg:string;
		knowledge_base_id:string;
		create_at:string;
		proc_status_json:string;
}