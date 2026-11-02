import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const editor = readFileSync(new URL('../wpi-editor.svelte', import.meta.url), 'utf8');

test('records node property changes before applying them, once per focus', () => {
 const update = editor.split('function updateNodeData(')[1].split('\n\t}')[0];
 assert.match(update, /recordEdit\(\);[^]*?nodes = nodes\.map/);
 assert.match(editor, /onPage=\{\(page\) => \{ recordEdit\(\); loadedPage = page; \}\}/);
});
test('records drag and delete before Svelte Flow changes the canvas', () => {
 assert.match(editor, /onnodedragstart=\{[^]*?takeSnapshot\(\)/);
 assert.match(editor, /onbeforedelete=\{[^]*?takeSnapshot\(\)/);
 assert.doesNotMatch(editor, /ondelete=\{\(\) => takeSnapshot\(\)\}/);
});
test('recognizes shifted redo shortcuts', () => {
 assert.match(editor, /e\.key\.toLowerCase\(\)/);
});
