import { z } from "zod/v4";

export const schema = z.object({
	id: z.number(),
	header: z.string(),
	type: z.string(),
	status: z.string(),
	target: z.string(),
	limit: z.string(),
	reviewer: z.string(),
});

export type Schema = z.infer<typeof schema>;

export const dashboard_01_schema = z.object({
	record_uuid: z.string(),
	md5: z.string(),
	doc_name: z.string(),
	doc_num: z.string(),
	file_url: z.string(),
	status: z.string(),
	data_proc_status: z.string(),
	// Add more fields as needed
});

export type Dashboard01Schema = z.infer<typeof dashboard_01_schema>;