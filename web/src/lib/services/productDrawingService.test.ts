import { describe, expect, test, mock } from 'bun:test';
import {
	composeDrawingPrompt,
	generateProductDrawing,
	ignoreProductDrawing,
	keepProductDrawing,
	pendingProductDrawingContentUrl
} from './productDrawingService';

function response(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('productDrawingService', () => {
	test('generates a pending drawing with the default subject', async () => {
		const fetchFn = mock().mockResolvedValue(
			response({ token: 'abc', image_url: '/api/image', prompt: 'prompt', model: 'model', expires_at: 'soon' }, 201)
		);
		const result = await generateProductDrawing(fetchFn);
		expect(result.token).toBe('abc');
		expect(fetchFn).toHaveBeenCalledWith('/api/v1/product-drawings/generate', expect.objectContaining({ method: 'POST' }));
	});

	test('keeps and ignores a pending drawing', async () => {
		const fetchFn = mock()
			.mockResolvedValueOnce(
				response({ status: true, filename: 'drawing.png', path: 'resources/product-drawings/drawing.png', id: 42 })
			)
			.mockResolvedValueOnce(response({ status: true, ignored: true }));
		await expect(keepProductDrawing('abc', fetchFn)).resolves.toMatchObject({ filename: 'drawing.png', id: 42 });
		await expect(ignoreProductDrawing('abc', fetchFn)).resolves.toBeUndefined();
		expect(pendingProductDrawingContentUrl('abc')).toBe('/api/v1/product-drawings/pending/abc/content');
	});

	test('composes a prompt from a product name and components', async () => {
		const fetchFn = mock().mockResolvedValue(response({ prompt: 'Draw a 3D exploded technical illustration of 血压计.' }));
		const result = await composeDrawingPrompt('血压计', ['控制按钮', '电路板'], fetchFn);
		expect(result.prompt).toBe('Draw a 3D exploded technical illustration of 血压计.');
		expect(fetchFn).toHaveBeenCalledWith(
			'/api/v1/product-drawings/compose-prompt',
			expect.objectContaining({
				method: 'POST',
				body: JSON.stringify({ product_name: '血压计', components: ['控制按钮', '电路板'] })
			})
		);
	});
});
