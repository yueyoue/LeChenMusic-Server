import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { authMiddleware, adminMiddleware } from '../../middleware/auth.js';
import { scannerService } from './scanner.service.js';
import { AppError } from '../../middleware/error-handler.js';
import { qs } from '../../utils/query.js';

const router = Router();

/** 获取媒体库列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const libraries = await db.select().from(schema.mediaLibrary).all();
    res.json({ code: 0, message: 'ok', data: libraries });
  } catch (err) { next(err); }
});

/** 添加媒体库（管理员） */
router.post('/', authMiddleware, adminMiddleware, async (req, res, next) => {
  try {
    const { name, storageType, storagePath } = req.body;
    if (!name || !storageType || !storagePath) {
      throw new AppError(2001, 400, 'Missing required fields: name, storageType, storagePath');
    }

    const result = await db.insert(schema.mediaLibrary).values({
      name,
      storageType,
      storagePath,
    }).returning().get();

    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

/** 触发媒体库扫描（管理员） */
router.post('/:id/scan', authMiddleware, adminMiddleware, async (req, res, next) => {
  try {
    const libraryId = parseInt(req.params.id as string);
    scannerService.scanLibrary(libraryId).catch(err => {
      console.error('Scan failed:', err);
    });
    res.json({ code: 0, message: 'Scan started', data: { libraryId } });
  } catch (err) { next(err); }
});

/** 删除媒体库（管理员） */
router.delete('/:id', authMiddleware, adminMiddleware, async (req, res, next) => {
  try {
    const libraryId = parseInt(req.params.id as string);
    await db.delete(schema.mediaLibrary).where(eq(schema.mediaLibrary.id, libraryId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

export default router;
