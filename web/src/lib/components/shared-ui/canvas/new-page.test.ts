import test from 'node:test';
import assert from 'node:assert/strict';
import { appSlug, newPageMetadata } from './new-page';

const size = { width: '1000px', height: '650px' };

test('slugs a name', () => {
 assert.equal(appSlug(' Chen App 01! '), 'chen-app-01');
 assert.equal(appSlug('Café  Menu'), 'cafe-menu');
 assert.equal(appSlug('我的应用'), '');
 assert.equal(appSlug('a'.repeat(70)).length, 64);
});

test('builds the page identity from the page name', () => {
 const metadata = newPageMetadata(' Chen Page 01 ', ' my first page ', ' tax, , audit ', ' client, priority ', size);
 assert.deepEqual(metadata, { descriptor_id: 'chen-page-01', page_name: 'Chen Page 01', description: 'my first page', keywords: ['tax', 'audit'], page_tags: ['client', 'priority'], width: '1000px', height: '650px' });
});

test('omits a blank optional description and embeds no app', () => {
 const metadata = newPageMetadata('Page', '  ', '', '', size);
 assert.equal('description' in metadata, false);
 assert.equal('app' in metadata, false);
});
