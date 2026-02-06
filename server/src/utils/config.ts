/**
 * 配置加载工具
 */

import dotenv from 'dotenv';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

dotenv.config({ path: path.resolve(__dirname, '../../.env') });

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

export function loadConfig(): Config {
  return {
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
}
