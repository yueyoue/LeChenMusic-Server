import type { StorageDriver, StorageType } from './types.js';
import { LocalDriver } from './local-driver.js';
import { logger } from '../../utils/logger.js';

export class StorageManager {
  private drivers = new Map<string, StorageDriver>();

  constructor() {
    // 注册默认的本地驱动
    this.register('local', new LocalDriver('/'));
  }

  register(key: string, driver: StorageDriver) {
    this.drivers.set(key, driver);
    logger.info({ key, type: driver.getType() }, 'Storage driver registered');
  }

  /**
   * 根据存储类型和路径获取对应的驱动
   * local: 直接用本地驱动，basePath 为库的 storagePath
   * smb/nfs: 用挂载的本地路径（推荐 rclone 或系统挂载）
   */
  getDriver(storageType: StorageType, storagePath: string): StorageDriver {
    if (storageType === 'local' || storageType === 'smb' || storageType === 'nfs') {
      // SMB/NFS 推荐通过系统挂载为本地目录，统一用 LocalDriver
      return new LocalDriver(storagePath);
    }
    throw new Error(`Unsupported storage type: ${storageType}`);
  }
}

export const storageManager = new StorageManager();
