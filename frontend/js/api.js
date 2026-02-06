/**
 * API 客户端封装
 */

class NominatimAPI {
  constructor(baseUrl = '') {
    this.baseUrl = baseUrl;
  }

  /**
   * 逆地理编码 (兼容 Nominatim 官方格式)
   */
  async reverse(lat, lon, params = {}) {
    const url = new URL(`${this.baseUrl}/api/reverse`);
    url.searchParams.set('lat', lat.toString());
    url.searchParams.set('lon', lon.toString());
    Object.entries(params).forEach(([key, value]) => {
      url.searchParams.set(key, value.toString());
    });

    const response = await fetch(url.toString());
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Request failed');
    }
    return response.json();
  }

  /**
   * 获取统计信息
   */
  async getStats() {
    const response = await fetch(`${this.baseUrl}/admin/stats`, {
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to get stats');
    }
    return result.data;
  }

  /**
   * 获取每日统计
   */
  async getDailyStats() {
    const response = await fetch(`${this.baseUrl}/admin/stats/daily`, {
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to get daily stats');
    }
    return result.data;
  }

  /**
   * 获取缓存列表
   */
  async getCacheList(params = {}) {
    const searchParams = new URLSearchParams();
    if (params.page) searchParams.set('page', params.page.toString());
    if (params.limit) searchParams.set('limit', params.limit.toString());
    if (params.search) searchParams.set('search', params.search);

    const response = await fetch(`${this.baseUrl}/admin/cache/list?${searchParams}`, {
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to get cache list');
    }
    return result;
  }

  /**
   * 获取单个缓存详情
   */
  async getCacheDetail(key) {
    const response = await fetch(`${this.baseUrl}/admin/cache/${encodeURIComponent(key)}`, {
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Cache not found');
    }
    return result.data;
  }

  /**
   * 删除缓存
   */
  async deleteCache(key) {
    const response = await fetch(`${this.baseUrl}/admin/cache/${encodeURIComponent(key)}`, {
      method: 'DELETE',
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to delete cache');
    }
    return result;
  }

  /**
   * 清空所有缓存
   */
  async clearAllCache() {
    const response = await fetch(`${this.baseUrl}/admin/cache/clear`, {
      method: 'POST',
      headers: this.getAuthHeaders()
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to clear cache');
    }
    return result;
  }

  /**
   * 查询上游 (返回多个源的结果)
   */
  async queryUpstream(lat, lon) {
    const response = await fetch(`${this.baseUrl}/admin/query`, {
      method: 'POST',
      headers: {
        ...this.getAuthHeaders(),
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ lat, lon })
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to query upstream');
    }
    return result;
  }

  /**
   * 手动添加/更新缓存
   */
  async addManualCache(data) {
    const response = await fetch(`${this.baseUrl}/admin/cache/add`, {
      method: 'POST',
      headers: {
        ...this.getAuthHeaders(),
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(data)
    });
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to add cache');
    }
    return result;
  }

  /**
   * 获取认证头
   */
  getAuthHeaders() {
    // 从浏览器存储获取认证信息，或使用默认
    const credentials = localStorage.getItem('nominatim_auth');
    if (credentials) {
      return {
        'Authorization': `Basic ${credentials}`
      };
    }
    // 默认 admin:admin123
    const defaultAuth = btoa('admin:admin123');
    localStorage.setItem('nominatim_auth', defaultAuth);
    return {
      'Authorization': `Basic ${defaultAuth}`
    };
  }
}

// 导出实例
const api = new NominatimAPI();
