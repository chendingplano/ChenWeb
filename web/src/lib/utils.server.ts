import * as fs from 'fs';

/**
 * Server-only utilities that require Node.js modules.
 * The .server.ts suffix ensures this is never bundled for the browser.
 */

export function checkConfigFileExists(configPath: string): boolean {
	return fs.existsSync(configPath);
}
