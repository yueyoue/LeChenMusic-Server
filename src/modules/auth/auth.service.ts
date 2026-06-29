import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import argon2 from 'argon2';
import jwt from 'jsonwebtoken';
import { config } from '../../config/index.js';
import { AppError } from '../../middleware/error-handler.js';
import type { JwtPayload } from '../../middleware/auth.js';

export class AuthService {
  async register(username: string, password: string, displayName?: string) {
    // 检查用户名是否已存在
    const existing = await db.select().from(schema.sysUser).where(eq(schema.sysUser.username, username)).get();
    if (existing) {
      throw new AppError(2001, 400, 'Username already exists');
    }

    // 检查是否是第一个用户（自动设为管理员）
    const userCount = await db.select().from(schema.sysUser).all();
    const role = userCount.length === 0 ? 'admin' : 'user';

    const passwordHash = await argon2.hash(password, { type: argon2.argon2id });

    const result = await db.insert(schema.sysUser).values({
      username,
      passwordHash,
      role,
      displayName: displayName || username,
    }).returning().get();

    return {
      id: result.id,
      username: result.username,
      role: result.role,
      displayName: result.displayName,
    };
  }

  async login(username: string, password: string) {
    const user = await db.select().from(schema.sysUser).where(eq(schema.sysUser.username, username)).get();
    if (!user) {
      throw new AppError(1003, 401, 'Invalid username or password');
    }

    const valid = await argon2.verify(user.passwordHash, password);
    if (!valid) {
      throw new AppError(1003, 401, 'Invalid username or password');
    }

    const payload: JwtPayload = {
      userId: user.id,
      username: user.username,
      role: user.role as 'admin' | 'user',
    };

    const accessToken = jwt.sign(payload, config.jwt.secret, {
      expiresIn: config.jwt.accessExpires,
    });

    const refreshToken = jwt.sign({ ...payload, type: 'refresh' }, config.jwt.secret, {
      expiresIn: config.jwt.refreshExpires,
    });

    return {
      accessToken,
      refreshToken,
      user: {
        id: user.id,
        username: user.username,
        role: user.role,
        displayName: user.displayName,
        avatar: user.avatar,
      },
    };
  }

  async refreshToken(refreshToken: string) {
    try {
      const payload = jwt.verify(refreshToken, config.jwt.secret) as JwtPayload & { type?: string };
      if (payload.type !== 'refresh') {
        throw new AppError(1004, 401, 'Invalid refresh token');
      }

      const newPayload: JwtPayload = {
        userId: payload.userId,
        username: payload.username,
        role: payload.role,
      };

      const newAccessToken = jwt.sign(newPayload, config.jwt.secret, {
        expiresIn: config.jwt.accessExpires,
      });

      return { accessToken: newAccessToken };
    } catch {
      throw new AppError(1004, 401, 'Invalid or expired refresh token');
    }
  }
}

export const authService = new AuthService();
