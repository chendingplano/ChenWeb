import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import ServerConfigLoader from '$lib/config/server-config-loader.server';

/**
 * GET /api/config
 * Returns the application configuration from config.toml
 *
 * This endpoint loads the config server-side and returns it as JSON
 * for the frontend to consume.
 */
export const GET: RequestHandler = async () => {
  try {
    const configLoader = ServerConfigLoader.getInstance();
    const config = configLoader.getAll();

    return json(config);
  } catch (error: any) {
    console.error('[API] Failed to load config:', error);
    return json(
      { error: 'Failed to load configuration', message: error.message },
      { status: 500 }
    );
  }
};
