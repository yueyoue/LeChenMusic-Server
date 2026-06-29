import type { Request, Response, NextFunction } from 'express';
import { logger } from '../utils/logger.js';

export class AppError extends Error {
  constructor(
    public code: number,
    public statusCode: number,
    message: string,
  ) {
    super(message);
    this.name = 'AppError';
  }
}

export function errorHandler(err: Error, req: Request, res: Response, _next: NextFunction) {
  if (err instanceof AppError) {
    return res.status(err.statusCode).json({
      code: err.code,
      message: err.message,
      data: null,
    });
  }

  logger.error({ err, url: req.url, method: req.method }, 'Unhandled error');

  return res.status(500).json({
    code: 9001,
    message: 'Internal server error',
    data: null,
  });
}
