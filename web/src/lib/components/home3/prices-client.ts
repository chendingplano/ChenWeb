export type PriceType = 'service' | 'llm';

export type PriceItem = {
	id?: number;
	item_name: string;
	item_type: 'input' | 'output';
	cache: '' | 'hit' | 'miss';
	time_span: '' | 'peak' | 'off-peak';
	unit: string;
	currency: string;
	// Decimal string, so prices keep their exact precision.
	value: string;
};

export type PriceDefInput = {
	price_def_name: string;
	price_type: PriceType;
	description: string;
	items: PriceItem[];
};

export type PriceDef = PriceDefInput & { id: number };

type Envelope = { status: boolean; results?: PriceDef[]; error_msg?: string };

async function request(url: string, init?: RequestInit): Promise<Envelope> {
	const response = await fetch(url, { credentials: 'same-origin', ...init });
	const body = await response.text();
	let parsed: Envelope | null = null;
	try {
		parsed = body ? (JSON.parse(body) as Envelope) : null;
	} catch {
		/* use HTTP status below */
	}
	if (!response.ok) throw new Error(parsed?.error_msg || `Request failed (${response.status})`);
	return parsed ?? { status: true };
}

export async function listPrices(): Promise<PriceDef[]> {
	return (await request('/api/v1/prices')).results ?? [];
}

export function createPrice(input: PriceDefInput): Promise<Envelope> {
	return request('/api/v1/prices', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function updatePrice(id: number, input: PriceDefInput): Promise<Envelope> {
	return request(`/api/v1/prices/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
}

export function deletePrice(id: number): Promise<Envelope> {
	return request(`/api/v1/prices/${id}`, { method: 'DELETE' });
}
