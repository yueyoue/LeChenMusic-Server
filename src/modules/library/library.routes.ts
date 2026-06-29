import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { authMiddleware, adminMiddleware } from '../../middleware/auth.js';
import { scannerService } from './scanner.service.js';
import { audiobookScannerService } from '../audiobook/audiobook-scanner.service.js';
import { AppError } from '../../middleware/error-handler.js';

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
    const { name, storageType, storagePath, mediaType } = req.body;
    if (!name || !storageType || !storagePath) {
      throw new AppError(2001, 400, 'Missing required fields: name, storageType, storagePath');
    }

    const result = await db.insert(schema.mediaLibrary).values({
      name,
      storageType,
      storagePath,
      mediaType: mediaType || 'music',
    }).returning().get();

    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

/** 触发媒体库扫描（管理员） */
router.post('/:id/scan', authMiddleware, adminMiddleware, async (req, res, next) => {
  try {
    const libraryId = parseInt(req.params.id as string);
    const library = await db.select().from(schema.mediaLibrary)
      .where(eq(schema.mediaLibrary.id, libraryId)).get();

    if (!library) throw new AppError(3001, 404, 'Library not found');

    // 根据媒体类型选择扫描器
    if (library.mediaType === 'audiobook') {
      audiobookScannerService.scanLibrary(libraryId).catch(err => console.error('Audiobook scan failed:', err));
    } else {
      scannerService.scanLibrary(libraryId).catch(err => console.error('Music scan failed:', err));
    }

    res.json({ code: 0, message: 'Scan started', data: { libraryId, mediaType: library.mediaType } });
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
