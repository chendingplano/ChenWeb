// Client for the image library (video covers) + AI cover generation.
// Backend: server/api/imagehandler; authenticated (same-origin) under /api/v1/images.

export type ImageMeta = {
	id: number;
	filename: string;
	size_bytes: number;
	content_type: string;
	origin: string; // 'upload' | 'generated'
	content_url: string;
	created_at: string;
};

/** List library images, newest first. */
export async function listImages(fetchFn: typeof fetch = fetch): Promise<ImageMeta[]> {
	const res = await fetchFn('/api/v1/images', { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`list images failed: ${res.status}`);
	return (await res.json()) as ImageMeta[];
}

/** Upload an image into the library. */
export async function uploadImage(file: File, fetchFn: typeof fetch = fetch): Promise<ImageMeta> {
	const form = new FormData();
	form.append('file', file);
	const res = await fetchFn('/api/v1/images', {
		method: 'POST',
		credentials: 'same-origin',
		body: form
	});
	if (!res.ok) throw new Error(await errorMessage(res, 'upload image failed'));
	return (await res.json()) as ImageMeta;
}

/** Generate a new cover image from a prompt (each call yields a new image). */
export async function generateImage(
	prompt: string,
	fetchFn: typeof fetch = fetch
): Promise<ImageMeta> {
	const res = await fetchFn('/api/v1/images/generate', {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ prompt })
	});
	if (!res.ok) throw new Error(await errorMessage(res, 'image generation failed'));
	return (await res.json()) as ImageMeta;
}

/** URL that serves an image's bytes. */
export function imageContentUrl(id: number): string {
	return `/api/v1/images/${id}/content`;
}

async function errorMessage(res: Response, fallback: string): Promise<string> {
	try {
		const body = (await res.json()) as { error?: string };
		if (body.error) return body.error;
	} catch {
		/* ignore */
	}
	return `${fallback}: ${res.status}`;
}
