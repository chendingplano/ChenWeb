import test from 'node:test';
import assert from 'node:assert/strict';
import { newPageMetadata } from './new-page';

test('builds a new page identity and normalizes metadata', () => {
 assert.deepEqual(newPageMetadata(' Customers ', ' All customers ', ' tax, , audit ', ' client, priority ', 'abc-123'), {
  descriptor_id: 'abc-123', page_name: 'Customers', description: 'All customers', keywords: ['tax', 'audit'], page_tags: ['client', 'priority']
 });
});

test('omits a blank optional description', () => {
 const metadata = newPageMetadata('Page', '  ', '', '', 'abc');
 assert.equal('description' in metadata, false);
});
