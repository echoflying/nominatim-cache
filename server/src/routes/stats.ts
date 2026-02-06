/**
 * 统计路由
 */

import { Router, Request, Response } from 'express';
import { getStats } from '../services/stats.js';

const router = Router();

/**
 * 获取统计信息
 * GET /admin/stats
 */
router.get('/', async (req: Request, res: Response): Promise<void> => {
  try {
    const stats = await getStats();

    res.json({
      success: true,
      data: stats
    });

  } catch (error) {
    console.error('[Stats] 获取统计失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to get stats'
    });
  }
});

export default router;
