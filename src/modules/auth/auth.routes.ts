import { Router } from 'express';
import { z } from 'zod';
import { authService } from './auth.service.js';
import { AppError } from '../../middleware/error-handler.js';

const router = Router();

const registerSchema = z.object({
  username: z.string().min(3).max(64),
  password: z.string().min(6).max(128),
  displayName: z.string().max(128).optional(),
});

const loginSchema = z.object({
  username: z.string(),
  password: z.string(),
});

const refreshSchema = z.object({
  refreshToken: z.string(),
});

/**
 * @openapi
 * /api/auth/register:
 *   post:
 *     summary: 用户注册
 *     tags: [Auth]
 *     requestBody:
 *       required: true
 *       content:
 *         application/json:
 *           schema:
 *             type: object
 *             properties:
 *               username:
 *                 type: string
 *               password:
 *                 type: string
 *               displayName:
 *                 type: string
 *     responses:
 *       200:
 *         description: 注册成功
 */
router.post('/register', async (req, res, next) => {
  try {
    const body = registerSchema.parse(req.body);
    const user = await authService.register(body.username, body.password, body.displayName);
    res.json({ code: 0, message: 'ok', data: user });
  } catch (err) {
    next(err);
  }
});

/**
 * @openapi
 * /api/auth/login:
 *   post:
 *     summary: 用户登录
 *     tags: [Auth]
 *     requestBody:
 *       required: true
 *       content:
 *         application/json:
 *           schema:
 *             type: object
 *             properties:
 *               username:
 *                 type: string
 *               password:
 *                 type: string
 *     responses:
 *       200:
 *         description: 登录成功
 */
router.post('/login', async (req, res, next) => {
  try {
    const body = loginSchema.parse(req.body);
    const result = await authService.login(body.username, body.password);
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) {
    next(err);
  }
});

/**
 * @openapi
 * /api/auth/refresh:
 *   post:
 *     summary: 刷新 Token
 *     tags: [Auth]
 *     requestBody:
 *       required: true
 *       content:
 *         application/json:
 *           schema:
 *             type: object
 *             properties:
 *               refreshToken:
 *                 type: string
 *     responses:
 *       200:
 *         description: Token 刷新成功
 */
router.post('/refresh', async (req, res, next) => {
  try {
    const body = refreshSchema.parse(req.body);
    const result = await authService.refreshToken(body.refreshToken);
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) {
    next(err);
  }
});

export default router;
