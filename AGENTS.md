# AGENTS.md - Coding Guidelines for Nominatim Cache Service

## Documentation Files Overview

| 文件 | 用途 | 读者 |
|------|------|------|
| **README.md** | 项目简介、快速开始、使用方法、配置说明 | 用户、外部调用者 |
| **docs/DESIGN.md** | 需求、架构设计、数据库设计、技术决策 | 开发者、AI Agent |
| **docs/API.md** | 接口详细文档、参数、响应格式 | 开发者、外部系统 |

### 文档更新原则

**修改代码前先更新文档**，确保文档与实现一致：
- 新增功能 → 更新 API.md 和 DESIGN.md
- 修改配置 → 更新 README.md
- 架构变化 → 更新 DESIGN.md
- 接口变更 → 更新 API.md

## Build, Lint, and Test Commands

### Server Commands (run from `/mnt/d/home.it/nominatim-cache/server`)

```bash
# Install dependencies
npm install

# Development (tsx hot-reload)
npm run dev

# TypeScript build (required before running)
npm run build

# Start production server
npm run start

# Initialize database
npm run db:init

# Run database migrations
npm run db:migrate

# Run tests (no tests configured - add tests before using)
npm run test
```

### Key Notes

- **Always run `npm run build` after TypeScript changes**
- The project uses `tsx` for development with hot-reload
- No test framework is configured - consider adding Jest or Vitest
- SQLite database: `server/data/cache.db`

## Code Style Guidelines

### TypeScript Conventions

#### Imports
```typescript
// Use .js extension for relative imports (ESM modules)
import { Router, Request, Response } from 'express';
import { getAllCaches } from '../services/cache.js';

// Use path aliases with @/
import { loadConfig } from '../utils/config.js';
```

#### Types and Interfaces
```typescript
// Use interfaces for object shapes
interface NominatimResponse {
  lat: string;
  lon: string;
  display_name: string;
  address: Record<string, any>;
  extratags?: Record<string, any>;
  type?: string;
  class?: string;
  error?: string;
}

// Use type for unions/primitives
type CacheResult = { data: any[]; total: number };
```

#### Naming Conventions
| Pattern | Example | Notes |
|---------|---------|-------|
| Interfaces | `NominatimResponse` | PascalCase |
| Types | `CacheResult` | PascalCase |
| Constants | `MAX_ATTEMPTS`, `BAN_DURATIONS` | UPPER_SNAKE_CASE |
| Variables | `cacheKey`, `pollingPointer` | camelCase |
| Functions | `fetchNominatim`, `getAllCaches` | camelCase |
| Private helpers | `isValidResponse`, `checkAndUnban` | camelCase |
| Database columns | `cache_key`, `display_name` | snake_case |

#### Function Declarations
```typescript
// Prefer async functions with Promise return type
export async function fetchAllUpstream(lat: number, lon: number): Promise<UpstreamResult[]> {
  // implementation
}

// Internal helpers can use function declarations
function buildParams(lat: number, lon: number, sourceName: string): Record<string, any> {
  // implementation
}
```

#### Route Handlers
```typescript
// Use explicit Promise<void> return type annotation
router.get('/cache/list', async (req: Request, res: Response): Promise<void> => {
  try {
    const result = await getAllCaches(page, limit, search);
    res.json({ success: true, data: result.data });
  } catch (error) {
    console.error('[Admin] Operation failed:', error);
    res.status(500).json({ success: false, error: 'Operation failed' });
  }
});
```

### Error Handling

```typescript
// Always wrap async operations in try-catch
try {
  const result = await getCacheEntry(cacheKey);
  if (!result) {
    res.status(404).json({ success: false, error: 'Not found' });
    return;
  }
  res.json({ success: true, data: result });
} catch (error) {
  console.error('[Service] Failed:', error);
  res.status(500).json({ success: false, error: 'Internal error' });
}

// Log errors with context
console.error('[Admin] 获取缓存列表失败:', error);
```

### Console Logging

```typescript
// Use prefixed log format for traceability
console.log(`[Nominatim] 上游返回: ${displayName}...`);
console.error('[Cache] 设置缓存失败:', error);

// Frontend: avoid console.log in production code
```

### Database (better-sqlite3)

```typescript
// Use prepared statements
const row = db.prepare(`SELECT * FROM cache_entries WHERE cache_key = ?`).get(cacheKey);
db.prepare(`INSERT INTO ...`).run(...params);

// Transactions for multi-operation
const db = getDatabase();
const transaction = db.transaction(() => {
  db.prepare(`INSERT INTO ...`).run(...);
  db.prepare(`UPDATE ...`).run(...);
});
transaction();
```

### Frontend JavaScript

```javascript
// Vanilla JS - no frameworks
document.addEventListener('DOMContentLoaded', () => {
  // initialization
});

// Use const/let, avoid var
const tableBody = document.getElementById('cacheTableBody');
let currentPage = 1;

// Event binding
document.querySelectorAll('.action-btn.delete').forEach(btn => {
  btn.addEventListener('click', () => handleDelete(btn.dataset.key));
});
```

### CSS Styling

```css
/* BEM-like naming */
.action-btn, .action-btn.delete, .modal-content;

/* Color variables in comments for consistency */
/* Primary: #0366d6, Success: #28a745, Danger: #cb2431 */
```

### Commit Style

- Use Chinese comments and log messages (project is Chinese)
- Keep commits focused and atomic
- Prefix logs with service name: `[Nominatim]`, `[Cache]`, `[Admin]`

### OSM Compliance (Important)

When calling upstream Nominatim services:

```typescript
// Always include required headers
headers: {
  'User-Agent': 'PhotoOrdo-Local/1.0 (echoflying@gmail.com)',
  'Referer': 'http://localhost:3000/',
  'Accept-Language': 'zh-CN,zh'
}

// Rate limiting: 1.5 seconds between calls
const UPSTREAM_INTERVAL = 1500;
```

## Summary

1. **Build**: `npm run build` after TypeScript changes
2. **Dev**: `npm run dev` for hot-reload development
3. **Style**: Strict TypeScript, ESM imports with .js extensions
4. **Naming**: PascalCase types, camelCase functions, UPPER_SNAKE constants
5. **Error Handling**: Always try-catch async operations, log with context
6. **OSM Compliance**: Include User-Agent header, respect rate limits
7. **Logging**: Use `[ServiceName]` prefix for traceability
