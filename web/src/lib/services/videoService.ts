// Client for the training-video management API (Resources > Videos > Training).
// Backend: server/api/videohandler; all endpoints authenticated (same-origin
// cookies) under /api/v1/videos.

export type VideoMeta = {
	id: number;
	filename: string;
	name: string;
	description: string;
	source: string;
	url: string;
	image_id: number | null;
	image_url: string;
	keywords: string;
	category: string;
	subcategory: string;
	container: string;
	status: string; // 'draft' | 'published' | 'archived'
	notes: string;
	video_type: string;
	size_bytes: number;
	content_type: string;
	uploaded_by: string;
	created_at: string;
};

export type VideoUploadFields = {
	name?: string;
	description?: string;
	source?: string; // 'Recording' | 'Web'
	url?: string;
	image_id?: number | null;
	keywords?: string;
	category?: string;
	subcategory?: string;
	container?: string;
	status?: string; // 'draft' | 'published' | 'archived'
	notes?: string;
	video_type?: string;
};

export type VideoListOptions = {
	sortBy?: 'name' | 'created_at' | 'size_bytes';
	sortDir?: 'asc' | 'desc';
	name?: string;
	timeFrom?: string; // YYYY-MM-DD
	timeTo?: string; // YYYY-MM-DD
};

/** List videos, optionally sorted/filtered server-side across the entire table. */
export async function listVideos(
	options: VideoListOptions = {},
	fetchFn: typeof fetch = fetch
): Promise<VideoMeta[]> {
	const params = new URLSearchParams();
	if (options.sortBy) params.set('sort_by', options.sortBy);
	if (options.sortDir) params.set('sort_dir', options.sortDir);
	if (options.name) params.set('name', options.name);
	if (options.timeFrom) params.set('time_from', options.timeFrom);
	if (options.timeTo) params.set('time_to', options.timeTo);
	const qs = params.toString();
	const res = await fetchFn(`/api/v1/videos${qs ? `?${qs}` : ''}`, { credentials: 'same-origin' });
	if (!res.ok) {
		let msg = `list videos failed: ${res.status}`;
		try {
			const body = (await res.json()) as { error?: string };
			if (body.error) msg = body.error;
		} catch {
			/* keep default message */
		}
		throw new Error(msg);
	}
	return (await res.json()) as VideoMeta[];
}

/**
 * Upload a single video file. `onProgress` (0–1) is reported when available.
 * Uses XMLHttpRequest so upload progress can be surfaced to the UI.
 */
export function uploadVideo(
	file: File,
	fields: VideoUploadFields = {},
	onProgress?: (fraction: number) => void
): Promise<VideoMeta> {
	return new Promise((resolve, reject) => {
		const form = new FormData();
		form.append('file', file);
		if (fields.name) form.append('name', fields.name);
		if (fields.description) form.append('description', fields.description);
		if (fields.source) form.append('source', fields.source);
		if (fields.url) form.append('url', fields.url);
		if (fields.image_id != null) form.append('image_id', String(fields.image_id));
		if (fields.keywords) form.append('keywords', fields.keywords);
		if (fields.category) form.append('category', fields.category);
		if (fields.subcategory) form.append('subcategory', fields.subcategory);
		if (fields.container) form.append('container', fields.container);
		if (fields.status) form.append('status', fields.status);
		if (fields.notes) form.append('notes', fields.notes);
		if (fields.video_type) form.append('video_type', fields.video_type);

		const xhr = new XMLHttpRequest();
		xhr.open('POST', '/api/v1/videos', true);
		xhr.withCredentials = true;

		if (onProgress && xhr.upload) {
			xhr.upload.onprogress = (e) => {
				if (e.lengthComputable) onProgress(e.loaded / e.total);
			};
		}

		xhr.onload = () => {
			if (xhr.status >= 200 && xhr.status < 300) {
				try {
					resolve(JSON.parse(xhr.responseText) as VideoMeta);
				} catch {
					reject(new Error('upload succeeded but response was not valid JSON'));
				}
			} else {
				let msg = `upload failed: ${xhr.status}`;
				try {
					const body = JSON.parse(xhr.responseText) as { error?: string };
					if (body.error) msg = body.error;
				} catch {
					/* keep default message */
				}
				reject(new Error(msg));
			}
		};
		xhr.onerror = () => reject(new Error('network error during upload'));
		xhr.send(form);
	});
}

/**
 * Update a video's metadata, and optionally replace its file. `onProgress`
 * (0–1) is reported when a replacement file is provided.
 */
export function updateVideo(
	id: number,
	fields: VideoUploadFields = {},
	file?: File | null,
	onProgress?: (fraction: number) => void
): Promise<VideoMeta> {
	return new Promise((resolve, reject) => {
		const form = new FormData();
		if (file) form.append('file', file);
		if (fields.name) form.append('name', fields.name);
		if (fields.description) form.append('description', fields.description);
		if (fields.source) form.append('source', fields.source);
		if (fields.url) form.append('url', fields.url);
		if (fields.image_id != null) form.append('image_id', String(fields.image_id));
		if (fields.keywords) form.append('keywords', fields.keywords);
		if (fields.category) form.append('category', fields.category);
		if (fields.subcategory) form.append('subcategory', fields.subcategory);
		if (fields.container) form.append('container', fields.container);
		if (fields.status) form.append('status', fields.status);
		if (fields.notes) form.append('notes', fields.notes);
		if (fields.video_type) form.append('video_type', fields.video_type);

		const xhr = new XMLHttpRequest();
		xhr.open('PATCH', `/api/v1/videos/${id}`, true);
		xhr.withCredentials = true;

		if (onProgress && xhr.upload) {
			xhr.upload.onprogress = (e) => {
				if (e.lengthComputable) onProgress(e.loaded / e.total);
			};
		}

		xhr.onload = () => {
			if (xhr.status >= 200 && xhr.status < 300) {
				try {
					resolve(JSON.parse(xhr.responseText) as VideoMeta);
				} catch {
					reject(new Error('update succeeded but response was not valid JSON'));
				}
			} else {
				let msg = `update failed: ${xhr.status}`;
				try {
					const body = JSON.parse(xhr.responseText) as { error?: string };
					if (body.error) msg = body.error;
				} catch {
					/* keep default message */
				}
				reject(new Error(msg));
			}
		};
		xhr.onerror = () => reject(new Error('network error during update'));
		xhr.send(form);
	});
}

/** Delete a video (metadata row + file). */
export async function deleteVideo(id: number, fetchFn: typeof fetch = fetch): Promise<void> {
	const res = await fetchFn(`/api/v1/videos/${id}`, {
		method: 'DELETE',
		credentials: 'same-origin'
	});
	if (!res.ok) throw new Error(`delete video failed: ${res.status}`);
}

/** URL for inline playback (supports range requests / seeking). */
export function videoStreamUrl(id: number): string {
	return `/api/v1/videos/${id}/stream`;
}

/** URL for downloading the original file as an attachment. */
export function videoDownloadUrl(id: number): string {
	return `/api/v1/videos/${id}/download`;
}
