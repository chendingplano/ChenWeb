import test from 'node:test';
import assert from 'node:assert/strict';
import { appSlug, newPageMetadata } from './new-page';

const size = { width: '1000px', height: '650px', schema_version: '0.5' };

test('slugs an app name', () => {
 assert.equal(appSlug(' Chen App 01! '), 'chen-app-01');
 assert.equal(appSlug('Café  Menu'), 'cafe-menu');
 assert.equal(appSlug('我的应用'), '');
 assert.equal(appSlug('a'.repeat(70)).length, 64);
});

test('builds the app attribute and the page identity from the app name', () => {
 const metadata = newPageMetadata(' Chen App 01 ', ' my first app ', ' tax, , audit ', ' client, priority ', size);
 const app = { description: 'my first app', descriptor_id: 'chen-app-01', width: '1000px', height: '650px', keywords: ['tax', 'audit'], page_name: 'chen-app-01-page', page_tags: ['client', 'priority'], schema_version: '0.5' };
 assert.deepEqual(metadata, { descriptor_id: 'chen-app-01', app });
});

test('omits a blank optional description', () => {
 const metadata = newPageMetadata('App', '  ', '', '', size);
 assert.equal('description' in metadata.app, false);
});
