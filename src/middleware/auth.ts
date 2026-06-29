import type { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import { config } from '../config/index.js';
import { AppError } from './error-handler.js';

export interface JwtPayload {
  userId: number;
  username: string;
  role: 'admin' | 'user';
}

declare global {
  namespace Express {
    interface Request {
      user?: JwtPayload;
    }
  }
}

export function authMiddleware(req: Request, _res: Response, next: NextFunction) {
  const authHeader = req.headers.authorization;
  if (!authHeader?.startsWith('Bearer ')) {
    throw new AppError(1001, 401, 'Missing or invalid authorization header');
  }

  const token = authHeader.slice(7);
  try {
    const payload = jwt.verify(token, config.jwt.secret) as JwtPayload;
    req.user = payload;
    next();
  } catch {
    throw new AppError(1002, 401, 'Token expired or invalid');
  }
}

export function adminMiddleware(req: Request, _res: Response, next: NextFunction) {
  if (req.user?.role !== 'admin') {
    throw new AppError(4001, 403, 'Admin access required');
  }
  next();
}
