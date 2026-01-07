/////////////////////////////////////////////////////////////////////
// ChenWeb/web/src/lib/config/server-config-loader.server.ts
// This module reads and parses the config.toml file into a structured
// TypeScript object for use in the application.
// It uses the 'toml' package to handle TOML parsing.
//
// IMPORTANT: The .server.ts suffix ensures this file is ONLY imported
// in server-side code and never bundled for the browser.
//
// Ensure you have the 'toml' package installed:
// npm install toml or bun add toml
// npm install --save-dev @types/toml (if type definitions are needed), or
// bun add -d @types/toml
//
// Usage in API endpoints:
// import { ServerConfigLoader } from '$lib/config/server-config-loader.server';
// const config = ServerConfigLoader.getInstance();
//
// Created by Chen Ding, 2026/01/02, by Qwen
// Modified: 2026/01/03 - Fixed browser compatibility issue
/////////////////////////////////////////////////////////////////////

import { readFileSync } from 'fs';
import { cwd } from 'process';
import toml from 'toml';
import { checkConfigFileExists } from '../utils.server';

// Interfaces matching your config structure
export interface AppConfig {
  app_name: string;
  debug: boolean;
  home_url: string;
}

export interface ServerConfig {
  port: number;
  host: string;
}

export interface DatabaseConfig {
  create_mysql: string;
  create_pg: string;
  pg_host: string;
  pg_port: number;
  pg_user_name: string;
  // pg_password excluded for security (not sent from backend)
  pg_db_name: string;
  mysql_host: string;
  mysql_port: number;
  mysql_user_name: string;
  // mysql_password excluded for security (not sent from backend)
  mysql_db_name: string;
  max_connections: number;
  database_type: 'pg' | 'mysql';
  need_create_tables: string;
}

export interface AppTableNamesConfig {
  table_name_documents: string;
  table_name_process_status: string;
  table_name_schedules: string;
}

export interface AuthConfig {
  // jwt_secret excluded for security (not sent from backend)
  session_duration_hours: number;
}

export interface FullConfig {
  app_name: string;
  debug: boolean;
  home_url: string;
  server: ServerConfig;
  database: DatabaseConfig;
  app_table_names: AppTableNamesConfig;
  auth: AuthConfig;
}

class ServerConfigLoader {
  private static instance: ServerConfigLoader | null = null;
  public readonly config: FullConfig = {} as FullConfig;

  private constructor() {
      const configPath = `${cwd()}/config.toml`;
      if (!checkConfigFileExists(configPath)) {
          const error_msg = `(CWB_CFG_080) Config file not found at path: ${configPath}`;
          console.error("***** Alarm:" + error_msg);
          throw new Error(error_msg);
      }
      try {
        const content = readFileSync(configPath, 'utf8');
        this.config = toml.parse(content) as FullConfig;
      } catch (err: any) {
        console.error(`[CONFIG] Failed to load config: ${err.message}`);
        throw new Error(`Config load failed: ${err.message}`);
      }
  }

  public static getInstance(): ServerConfigLoader {
    if (!ServerConfigLoader.instance) {
      ServerConfigLoader.instance = new ServerConfigLoader();
    }
    return ServerConfigLoader.instance;
  }

  // Getter methods for top-level fields
  getAppName(): string {
    return this.config.app_name;
  }

  getDebug(): boolean {
    return this.config.debug;
  }

  getHomeUrl(): string {
    return this.config.home_url;
  }

  // Section getters
  getServerConfig(): ServerConfig {
    return this.config.server;
  }

  getDatabaseConfig(): DatabaseConfig {
    return this.config.database;
  }

  getTableNames(): AppTableNamesConfig {
    return this.config.app_table_names;
  }

  getTableNameDocuments(): string {
    return this.config.app_table_names.table_name_process_status;
  }

  getTableNameProcessStatus(): string {
    return this.config.app_table_names.table_name_process_status;
  }

  getTableNameSchedules(): string {
    return this.config.app_table_names.table_name_schedules;
  }

  getAuthConfig(): AuthConfig {
    return this.config.auth;
  }

  // Convenience method to get full config (use sparingly)
  getAll(): FullConfig {
    return this.config;
  }
}

export default ServerConfigLoader;
