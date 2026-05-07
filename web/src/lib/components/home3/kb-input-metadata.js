// @ts-nocheck

/**
 * @typedef {'text' | 'textarea' | 'datetime' | 'array' | 'json'} MetadataEditorKind
 * @typedef {{
 *   label: string;
 *   key: string;
 *   value: string;
 *   rawValue: unknown;
 *   editable: boolean;
 *   editor?: MetadataEditorKind;
 *   editKey?: string;
 *   wide?: boolean;
 *   pathLike?: boolean;
 * }} MetadataRow
 */

/**
 * @param {unknown} value
 * @returns {string}
 */
function safeJsonStringify(value) {
	try {
		return JSON.stringify(value);
	} catch {
		return String(value);
	}
}

/**
 * @param {unknown} value
 * @returns {unknown}
 */
function normalizeJsonLike(value) {
	if (typeof value !== 'string') return value;
	const trimmed = value.trim();
	if (!trimmed) return '';
	if (
		(trimmed.startsWith('{') && trimmed.endsWith('}')) ||
		(trimmed.startsWith('[') && trimmed.endsWith(']'))
	) {
		try {
			return JSON.parse(trimmed);
		} catch {
			return value;
		}
	}
	return value;
}

/**
 * @param {unknown} node
 * @param {WeakSet<object>} [seen]
 * @returns {unknown}
 */
function findFirstDocMetadata(node, seen = new WeakSet()) {
	const normalized = normalizeJsonLike(node);
	if (normalized === null || normalized === undefined) return null;
	if (typeof normalized !== 'object') return null;

	if (Array.isArray(normalized)) {
		for (const item of normalized) {
			const found = findFirstDocMetadata(item, seen);
			if (found != null) return found;
		}
		return null;
	}

	const obj = /** @type {Record<string, unknown>} */ (normalized);
	if (seen.has(obj)) return null;
	seen.add(obj);

	if (obj.doc_metadata != null) return obj.doc_metadata;
	if (obj.docMetadata != null) return obj.docMetadata;
	if (obj.document_metadata != null) return obj.document_metadata;
	if (obj.documentMetadata != null) return obj.documentMetadata;

	for (const value of Object.values(obj)) {
		const found = findFirstDocMetadata(value, seen);
		if (found != null) return found;
	}
	return null;
}

/**
 * @param {unknown} value
 * @returns {unknown}
 */
function extractDocMetadataFromUnknown(value) {
	const normalized = normalizeJsonLike(value);
	if (normalized == null) return null;
	return findFirstDocMetadata(normalized);
}

/**
 * @param {unknown} value
 * @param {string} [path]
 * @param {Array<{ key: string; value: string; rawValue: unknown }>} [out]
 * @param {WeakSet<object>} [seen]
 * @returns {Array<{ key: string; value: string; rawValue: unknown }>}
 */
function flattenDocMetadata(value, path = 'root', out = [], seen = new WeakSet()) {
	if (value === null || value === undefined) {
		out.push({ key: path, value: 'null', rawValue: null });
		return out;
	}
	if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
		out.push({ key: path, value: String(value), rawValue: value });
		return out;
	}
	if (Array.isArray(value)) {
		if (value.length === 0) {
			out.push({ key: path, value: '[]', rawValue: [] });
			return out;
		}
		const isStringArray = value.every((item) => typeof item === 'string');
		if (isStringArray) {
			out.push({ key: path, value: JSON.stringify(value), rawValue: value });
			return out;
		}
		for (let i = 0; i < value.length; i += 1) {
			flattenDocMetadata(value[i], `${path}[${i}]`, out, seen);
		}
		return out;
	}
	if (typeof value === 'object') {
		const obj = /** @type {Record<string, unknown>} */ (value);
		if (seen.has(obj)) {
			out.push({ key: path, value: '[Circular]', rawValue: '[Circular]' });
			return out;
		}
		seen.add(obj);
		const entries = Object.entries(obj).sort(([a], [b]) => a.localeCompare(b));
		if (entries.length === 0) {
			out.push({ key: path, value: '{}', rawValue: {} });
			return out;
		}
		for (const [k, v] of entries) {
			const childPath = path === 'root' ? k : `${path}.${k}`;
			flattenDocMetadata(v, childPath, out, seen);
		}
		return out;
	}
	out.push({ key: path, value: safeJsonStringify(value), rawValue: value });
	return out;
}

/**
 * @param {unknown} value
 * @returns {string}
 */
function asDisplayText(value) {
	if (value === null || value === undefined) return '—';
	if (typeof value === 'string') return value || '—';
	if (typeof value === 'number' || typeof value === 'boolean') return String(value);
	return safeJsonStringify(value);
}

/**
 * @param {string | undefined} value
 * @returns {string[]}
 */
function parseAuthors(value) {
	if (!value) return [];
	const trimmed = value.trim();
	if (!trimmed) return [];
	try {
		const parsed = JSON.parse(trimmed);
		if (Array.isArray(parsed)) {
			return parsed
				.map((item) => (typeof item === 'string' ? item.trim() : String(item)))
				.filter((item) => item.length > 0);
		}
	} catch {
		// fallback to plain text
	}
	return trimmed
		.split(/[,\n]/)
		.map((item) => item.trim())
		.filter((item) => item.length > 0);
}

/**
 * @param {string | undefined} value
 * @returns {string}
 */
function formatMaybeDate(value) {
	if (!value) return '—';
	return value.replace('T', ' ').slice(0, 19);
}

/**
 * @param {string | undefined} value
 * @returns {string}
 */
function toDateTimeLocalInput(value) {
	if (!value) return '';
	const d = new Date(value);
	if (Number.isNaN(d.getTime())) return '';
	const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
	return local.toISOString().slice(0, 16);
}

/**
 * @param {string} value
 * @returns {string | null}
 */
function fromDateTimeLocalInput(value) {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const d = new Date(trimmed);
	if (Number.isNaN(d.getTime())) return null;
	return d.toISOString();
}

/**
 * @param {unknown} value
 * @returns {string}
 */
export function safePrettyJson(value) {
	try {
		return JSON.stringify(value ?? null, null, 2);
	} catch {
		return safeJsonStringify(value);
	}
}

/**
 * @param {KbInputRecord | null} record
 * @returns {unknown}
 */
export function getKbInputDocMetadataValue(record) {
	if (!record) return null;
	const candidateRecord = /** @type {KbInputRecord & { docMetadata?: unknown }} */ (record);
	const topLevel = candidateRecord.doc_metadata ?? candidateRecord.docMetadata;
	if (topLevel != null) return topLevel;

	const fromPublicInfo = extractDocMetadataFromUnknown(record.public_info);
	if (fromPublicInfo != null) return fromPublicInfo;

	const fromPrivateInfo = extractDocMetadataFromUnknown(record.private_info);
	if (fromPrivateInfo != null) return fromPrivateInfo;

	return null;
}

/**
 * @param {KbInputRecord | null} currentInput
 * @returns {MetadataRow[]}
 */
export function buildKbInputRecordMetadataRows(currentInput) {
	if (!currentInput) return [];
	const authorsArray = parseAuthors(currentInput.authors);
	return [
		{ label: 'ID', key: 'id', value: String(currentInput.id), rawValue: currentInput.id, editable: false, editKey: 'field:id' },
		{ label: 'Type', key: 'type', value: currentInput.type || '—', rawValue: currentInput.type, editable: false, editKey: 'field:type' },
		{ label: 'Name', key: 'name', value: currentInput.name || '—', rawValue: currentInput.name, editable: false, editKey: 'field:name' },
		{ label: 'Title', key: 'title', value: currentInput.title || '—', rawValue: currentInput.title ?? '', editable: true, editor: 'text', editKey: 'field:title' },
		{ label: 'Doc No', key: 'doc_no', value: currentInput.doc_no || '—', rawValue: currentInput.doc_no ?? '', editable: true, editor: 'text', editKey: 'field:doc_no' },
		{ label: 'Source', key: 'source', value: currentInput.source || '—', rawValue: currentInput.source ?? '', editable: true, editor: 'text', editKey: 'field:source' },
		{ label: 'Published', key: 'publish_date', value: formatMaybeDate(currentInput.publish_date), rawValue: toDateTimeLocalInput(currentInput.publish_date), editable: true, editor: 'datetime', editKey: 'field:publish_date' },
		{ label: 'Authors', key: 'authors', value: authorsArray.length > 0 ? authorsArray.join(', ') : '—', rawValue: authorsArray, editable: true, editor: 'array', editKey: 'field:authors' },
		{ label: 'Owner', key: 'owner', value: currentInput.owner == null ? '—' : String(currentInput.owner), rawValue: currentInput.owner == null ? '' : String(currentInput.owner), editable: true, editor: 'text', editKey: 'field:owner' },
		{ label: 'Public Info', key: 'public_info', value: asDisplayText(currentInput.public_info), rawValue: currentInput.public_info ?? null, editable: true, editor: 'json', editKey: 'field:public_info' },
		{ label: 'Private Info', key: 'private_info', value: asDisplayText(currentInput.private_info), rawValue: currentInput.private_info ?? null, editable: true, editor: 'json', editKey: 'field:private_info' },
		{ label: 'File', key: 'file_name', value: currentInput.file_name || '—', rawValue: currentInput.file_name, editable: false, editKey: 'field:file_name' },
		{ label: 'Result File', key: 'result_filename', value: currentInput.result_filename || '—', rawValue: currentInput.result_filename, editable: false, editKey: 'field:result_filename' },
		{ label: 'Notes', key: 'notes', value: currentInput.notes || '—', rawValue: currentInput.notes ?? '', editable: true, editor: 'textarea', editKey: 'field:notes' },
		{ label: 'Error', key: 'error_msg', value: currentInput.error_msg || '—', rawValue: currentInput.error_msg ?? '', editable: true, editor: 'textarea', editKey: 'field:error_msg' },
		{ label: 'Created', key: 'create_time', value: formatMaybeDate(currentInput.create_time), rawValue: currentInput.create_time, editable: false, editKey: 'field:create_time' },
		{ label: 'Updated', key: 'modify_time', value: formatMaybeDate(currentInput.modify_time), rawValue: currentInput.modify_time, editable: false, editKey: 'field:modify_time' }
	];
}

/**
 * @param {KbInputRecord | null} currentInput
 * @returns {MetadataRow[]}
 */
export function buildKbInputDocMetadataRows(currentInput) {
	const raw = getKbInputDocMetadataValue(currentInput);
	if (raw == null) return [];

	if (typeof raw === 'string') {
		const trimmed = raw.trim();
		if (!trimmed) return [];
		try {
			const parsed = JSON.parse(trimmed);
			return flattenDocMetadata(parsed).map((row) => ({
				label: row.key,
				key: row.key,
				value: row.value,
				rawValue: row.rawValue,
				editor: typeof row.rawValue === 'object' && row.rawValue !== null ? 'json' : 'text',
				editable: true,
				editKey: `docmeta:${row.key}`,
				wide: true,
				pathLike: true
			}));
		} catch {
			return [{
				label: 'doc_metadata',
				key: 'doc_metadata',
				value: raw,
				rawValue: raw,
				editor: 'text',
				editable: true,
				editKey: 'docmeta:doc_metadata',
				wide: true,
				pathLike: true
			}];
		}
	}

	return flattenDocMetadata(raw).map((row) => ({
		label: row.key,
		key: row.key,
		value: row.value,
		rawValue: row.rawValue,
		editor: typeof row.rawValue === 'object' && row.rawValue !== null ? 'json' : 'text',
		editable: true,
		editKey: `docmeta:${row.key}`,
		wide: true,
		pathLike: true
	}));
}

/**
 * @param {string} path
 * @returns {Array<string | number>}
 */
function parsePathSegments(path) {
	if (!path || path === 'root') return [];
	const segments = [];
	const re = /([^[.\]]+)|\[(\d+)\]/g;
	let match = null;
	while (true) {
		match = re.exec(path);
		if (!match) break;
		if (match[1]) {
			segments.push(match[1]);
		} else if (match[2]) {
			segments.push(Number(match[2]));
		}
	}
	return segments;
}

/**
 * @template T
 * @param {T} value
 * @returns {T}
 */
function deepCloneValue(value) {
	try {
		return structuredClone(value);
	} catch {
		try {
			return JSON.parse(JSON.stringify(value));
		} catch {
			return value;
		}
	}
}

/**
 * @param {unknown} root
 * @param {string} path
 * @returns {unknown}
 */
function getValueByPath(root, path) {
	if (!path || path === 'root') return root;
	const segments = parsePathSegments(path);
	let cur = root;
	for (const seg of segments) {
		if (cur == null || typeof cur !== 'object') return undefined;
		if (typeof seg === 'number') {
			if (!Array.isArray(cur)) return undefined;
			cur = cur[seg];
		} else {
			cur = /** @type {Record<string, unknown>} */ (cur)[seg];
		}
	}
	return cur;
}

/**
 * @param {unknown} root
 * @param {string} path
 * @param {unknown} value
 * @returns {unknown}
 */
function setValueByPath(root, path, value) {
	if (path === 'root') return value;
	const base = root && typeof root === 'object' ? deepCloneValue(root) : {};
	const segments = parsePathSegments(path);
	if (segments.length === 0) return value;

	let cursor = base;
	for (let i = 0; i < segments.length; i += 1) {
		const segment = segments[i];
		const isLast = i === segments.length - 1;
		const next = segments[i + 1];

		if (typeof segment === 'number') {
			if (!Array.isArray(cursor)) return base;
			if (isLast) {
				cursor[segment] = value;
				break;
			}
			if (cursor[segment] == null || typeof cursor[segment] !== 'object') {
				cursor[segment] = typeof next === 'number' ? [] : {};
			}
			cursor = cursor[segment];
			continue;
		}

		if (cursor == null || typeof cursor !== 'object' || Array.isArray(cursor)) return base;
		const obj = /** @type {Record<string, unknown>} */ (cursor);
		if (isLast) {
			obj[segment] = value;
			break;
		}
		if (obj[segment] == null || typeof obj[segment] !== 'object') {
			obj[segment] = typeof next === 'number' ? [] : {};
		}
		cursor = obj[segment];
	}
	return base;
}

/**
 * @param {string} raw
 * @param {unknown} original
 * @returns {unknown}
 */
function parseScalarWithHint(raw, original) {
	const trimmed = raw.trim();
	if (original === null) {
		if (trimmed === '' || trimmed === 'null') return null;
	}
	if (typeof original === 'number') {
		const n = Number(trimmed);
		return Number.isNaN(n) ? raw : n;
	}
	if (typeof original === 'boolean') {
		if (trimmed.toLowerCase() === 'true') return true;
		if (trimmed.toLowerCase() === 'false') return false;
		return original;
	}
	return raw;
}

/**
 * @param {string} draft
 * @returns {string[]}
 */
function parseAuthorsDraft(draft) {
	return draft
		.split(/\r?\n|,/)
		.map((item) => item.trim())
		.filter((item) => item.length > 0);
}

/**
 * @param {KbInputRecord} currentInput
 * @param {MetadataRow} row
 * @param {string} draft
 * @param {MetadataEditorKind} editor
 * @returns {Record<string, unknown>}
 */
export function buildKbInputUpdatePayloadForMetadataEdit(currentInput, row, draft, editor) {
	const editKey = row.editKey ?? row.key;
	const payload = {};
	if (editKey.startsWith('field:')) {
		const field = editKey.replace(/^field:/, '');
		switch (field) {
			case 'title':
			case 'doc_no':
			case 'source':
				payload[field] = draft.trim();
				break;
			case 'publish_date': {
				const iso = fromDateTimeLocalInput(draft);
				if (draft.trim() && !iso) {
					throw new Error('Publish date is invalid.');
				}
				payload.publish_date = iso;
				break;
			}
			case 'authors':
				payload.authors = parseAuthorsDraft(draft);
				break;
			case 'owner': {
				const trimmed = draft.trim();
				payload.owner = trimmed === '' ? null : trimmed;
				break;
			}
			case 'public_info':
			case 'private_info': {
				const text = draft.trim();
				payload[field] = text ? JSON.parse(text) : null;
				break;
			}
			case 'notes':
			case 'error_msg':
				payload[field] = draft;
				break;
			default:
				throw new Error('Unsupported editable field.');
		}
		return payload;
	}

	if (editKey.startsWith('docmeta:')) {
		const path = editKey.replace(/^docmeta:/, '');
		const currentDocMeta = getKbInputDocMetadataValue(currentInput);
		const previousValue = getValueByPath(currentDocMeta, path);
		let nextValue = draft;
		if (editor === 'json') {
			const text = draft.trim();
			nextValue = text ? JSON.parse(text) : null;
		} else {
			nextValue = parseScalarWithHint(draft, previousValue);
		}
		payload.doc_metadata = setValueByPath(currentDocMeta ?? {}, path, nextValue);
		return payload;
	}

	throw new Error('Unsupported editable field.');
}
