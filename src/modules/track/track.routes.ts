import { Router } from 'express';
import { trackService } from './track.service.js';
import { authMiddleware } from '../../middleware/auth.js';

const router = Router();

/**
 * @openapi
 * /api/tracks:
 *   get:
 *     summary: 获取音轨列表
 *     tags: [Tracks]
 *     security:
 *       - bearerAuth: []
 *     parameters:
 *       - in: query
 *         name: page
 *         schema: { type: integer, default: 1 }
 *       - in: query
 *         name: pageSize
 *         schema: { type: integer, default: 50 }
 *       - in: query
 *         name: search
 *         schema: { type: string }
 *       - in: query
 *         name: artistId
 *         schema: { type: integer }
 *       - in: query
 *         name: albumId
 *         schema: { type: integer }
 *       - in: query
 *         name: genre
 *         schema: { type: string }
 *     responses:
 *       200:
 *         description: 音轨列表
 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const result = await trackService.list({
      page: parseInt(req.query.page as string) || 1,
      pageSize: parseInt(req.query.pageSize as string) || 50,
      search: req.query.search as string,
      artistId: req.query.artistId ? parseInt(req.query.artistId as string) : undefined,
      albumId: req.query.albumId ? parseInt(req.query.albumId as string) : undefined,
      genre: req.query.genre as string,
      orderBy: req.query.orderBy as any,
      order: req.query.order as any,
    });
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) {
    next(err);
  }
});

/**
 * @openapi
 * /api/tracks/{id}:
 *   get:
 *     summary: 获取音轨详情
 *     tags: [Tracks]
 *     security:
 *       - bearerAuth: []
 *     parameters:
 *       - in: path
 *         name: id
 *         required: true
 *         schema: { type: integer }
 *     responses:
 *       200:
 *         description: 音轨详情
 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const track = await trackService.getById(parseInt(req.params.id));
    res.json({ code: 0, message: 'ok', data: track });
  } catch (err) {
    next(err);
  }
});

/**
 * @openapi
 * /api/tracks/{id}/stream:
 *   get:
 *     summary: 流式播放音轨
 *     tags: [Tracks]
 *     security:
 *       - bearerAuth: []
 *     parameters:
 *       - in: path
 *         name: id
 *         required: true
 *         schema: { type: integer }
 *       - in: header
 *         name: Range
 *         schema: { type: string }
 *     responses:
 *       200:
 *         description: 音频流
 *       206:
 *         description: 部分音频流
 */
router.get('/:id/stream', authMiddleware, async (req, res, next) => {
  try {
    const { stream, headers, status } = await trackService.stream(
      parseInt(req.params.id),
      req.headers.range,
    );

    res.status(status);
    Object.entries(headers).forEach(([k, v]) => res.setHeader(k, v));
    stream.pipe(res);

    req.on('close', () => stream.destroy());
  } catch (err) {
    next(err);
  }
});

export default router;
