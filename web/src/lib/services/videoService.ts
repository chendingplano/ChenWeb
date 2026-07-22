// Client for the training-video management API (Resources > Videos > Training).
// Backend: server/api/videohandler; all endpoints authenticated (same-origin
// cookies) under /api/v1/videos.

export type VideoMeta = {
	id: number;
	filename: string;
	size_bytes: number;
	content_type: string;
	uploaded_by: string;
	created_at: string;
};

/** List all videos, newest first. */
export async function listVideos(fetchFn: typeof fetch = fetch): Promise<VideoMeta[]> {
	const res = await fetchFn('/api/v1/videos', { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`list videos failed: ${res.status}`);
	return (await res.json()) as VideoMeta[];
}

/**
 * Upload a single video file. `onProgress` (0–1) is reported when available.
 * Uses XMLHttpRequest so upload progress can be surfaced to the UI.
 */
export function uploadVideo(
	file: File,
	onProgress?: (fraction: number) => void
): Promise<VideoMeta> {
	return new Promise((resolve, reject) => {
		const form = new FormData();
		form.append('file', file);

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
