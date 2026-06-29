import { Router } from 'express';
import { trackService } from './track.service.js';
import { authMiddleware } from '../../middleware/auth.js';
import { qs, qn } from '../../utils/query.js';

const router = Router();

/** 获取音轨列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const result = await trackService.list({
      page: qn(req.query.page, 1),
      pageSize: qn(req.query.pageSize, 50),
      search: qs(req.query.search),
      artistId: qn(req.query.artistId),
      albumId: qn(req.query.albumId),
      genre: qs(req.query.genre),
      orderBy: qs(req.query.orderBy) as any,
      order: qs(req.query.order) as any,
    });
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

/** 获取音轨详情 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const track = await trackService.getById(parseInt(req.params.id as string));
    res.json({ code: 0, message: 'ok', data: track });
  } catch (err) { next(err); }
});

/** 流式播放音轨 */
router.get('/:id/stream', authMiddleware, async (req, res, next) => {
  try {
    const { stream, headers, status } = await trackService.stream(
      parseInt(req.params.id as string),
      req.headers.range,
    );

    res.status(status);
    Object.entries(headers).forEach(([k, v]) => res.setHeader(k, v));
    stream.pipe(res);

    req.on('close', () => stream.destroy());
  } catch (err) { next(err); }
});

export default router;
