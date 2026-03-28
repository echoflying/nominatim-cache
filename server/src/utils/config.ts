/**
 * 配置加载工具
 */

import dotenv from 'dotenv';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

let loadedEnv = false;

export interface Config {
  PORT: number;
  NODE_ENV: string;
  DATABASE_PATH: string;
  ADMIN_USERNAME: string;
  ADMIN_PASSWORD: string;
  NOMINATIM_PRIMARY: string;
  NOMINATIM_BACKUP_1: string;
  NOMINATIM_BACKUP_2: string;
  LOG_RETENTION_DAYS: number;
  REQUEST_INTERVAL: number;
}

export function getEnvFileCandidates(baseDir: string = __dirname): string[] {
  return [
    path.resolve(baseDir, '../../../.env'),
    path.resolve(baseDir, '../../.env')
  ];
}

function loadEnvFiles(): void {
  if (loadedEnv) {
    return;
  }

  for (const envPath of getEnvFileCandidates()) {
    if (fs.existsSync(envPath)) {
      dotenv.config({ path: envPath });
    }
  }

  loadedEnv = true;
}

function ensureProductionCredentials(adminUsername: string, adminPassword: string): void {
  const isDefaultUsername = adminUsername === 'admin';
  const isDefaultPassword = adminPassword === 'admin123';

  if (process.env.NODE_ENV === 'production' && (isDefaultUsername || isDefaultPassword)) {
    throw new Error('[Config] 生产环境必须显式设置非默认的 ADMIN_USERNAME 和 ADMIN_PASSWORD');
  }
}

export function loadConfig(): Config {
  loadEnvFiles();

  const config = {
    PORT: parseInt(process.env.PORT || '3000', 10),
    NODE_ENV: process.env.NODE_ENV || 'development',
    DATABASE_PATH: process.env.DATABASE_PATH || './data/cache.db',
    ADMIN_USERNAME: process.env.ADMIN_USERNAME || 'admin',
    ADMIN_PASSWORD: process.env.ADMIN_PASSWORD || 'admin123',
    NOMINATIM_PRIMARY: process.env.NOMINATIM_PRIMARY || 'https://api.mirror-earth.com/nominatim/reverse',
    NOMINATIM_BACKUP_1: process.env.NOMINATIM_BACKUP_1 || 'https://photon.komoot.io/reverse',
    NOMINATIM_BACKUP_2: process.env.NOMINATIM_BACKUP_2 || 'https://nominatim.openstreetmap.org/reverse',
    LOG_RETENTION_DAYS: parseInt(process.env.LOG_RETENTION_DAYS || '30', 10),
    REQUEST_INTERVAL: parseInt(process.env.REQUEST_INTERVAL || '1000', 10)
  };

  ensureProductionCredentials(config.ADMIN_USERNAME, config.ADMIN_PASSWORD);

  return config;
}
