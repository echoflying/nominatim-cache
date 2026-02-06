/**
 * Basic Auth 认证中间件
 */

import { Request, Response, NextFunction } from 'express';
import auth from 'basic-auth';
import { loadConfig } from '../utils/config.js';

const config = loadConfig();

export function authMiddleware(req: Request, res: Response, next: NextFunction): void {
  const credentials = auth(req);

  if (!credentials) {
    res.set('WWW-Authenticate', 'Basic realm="Nominatim Cache Admin"');
    res.status(401).json({
      success: false,
      error: 'Authentication required'
    });
    return;
  }

  const { name: username, pass: password } = credentials;

  if (username === config.ADMIN_USERNAME && password === config.ADMIN_PASSWORD) {
    next();
  } else {
    res.set('WWW-Authenticate', 'Basic realm="Nominatim Cache Admin"');
    res.status(401).json({
      success: false,
      error: 'Invalid credentials'
    });
  }
}
