import test from 'node:test';
import assert from 'node:assert/strict';
import { uploadPageImage } from './page-image';

test('uploads the PNG to the page-images endpoint and returns the stored name', async () => {
	const image = new Blob(['png'], { type: 'image/png' });
	const fetchFn = (async (url, init) => {
		assert.equal(url, '/api/v1/wpi/page-images/customers');
		assert.equal(init?.method, 'PUT');
		assert.equal((init?.headers as Record<string, string>)['Content-Type'], 'image/png');
		assert.equal(init?.body, image);
		return Response.json({ page_image: 'customers-1-ab.png' });
	}) as typeof fetch;
	assert.equal(await uploadPageImage('customers', image, fetchFn), 'customers-1-ab.png');
});

test('reports the server error when the upload is refused', async () => {
	const fetchFn = (async () => Response.json({ error: 'DATA_HOME_DIR is not set' }, { status: 503 })) as typeof fetch;
	await assert.rejects(() => uploadPageImage('x', new Blob([]), fetchFn), /DATA_HOME_DIR/);
});
