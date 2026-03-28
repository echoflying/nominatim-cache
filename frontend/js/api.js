/**
 * API 客户端封装
 */

class NominatimAPI {
  constructor(baseUrl = '') {
    this.baseUrl = baseUrl;
    this.authStorageKey = 'nominatim_auth';
    this.authPromptMessage = '请输入管理后台账号和密码';
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/stats`);
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/stats/daily`);
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

    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/cache/list?${searchParams}`);
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/cache/${encodeURIComponent(key)}`);
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/cache/${encodeURIComponent(key)}`, {
      method: 'DELETE',
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/cache/clear`, {
      method: 'POST',
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/query`, {
      method: 'POST',
      headers: {
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
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/cache/add`, {
      method: 'POST',
      headers: {
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
  async authenticatedFetch(url, options = {}, allowRetry = true) {
    const response = await fetch(url, {
      ...options,
      headers: {
        ...this.getAuthHeaders(),
        ...(options.headers || {})
      }
    });

    if (response.status === 401 && allowRetry) {
      this.clearStoredAuth();
      const updated = this.ensureCredentials(true);
      if (updated) {
        return this.authenticatedFetch(url, options, false);
      }
    }

    return response;
  }

  getAuthHeaders() {
    const credentials = this.ensureCredentials();
    return credentials
      ? { 'Authorization': `Basic ${credentials}` }
      : {};
  }

  ensureCredentials(forcePrompt = false) {
    let credentials = forcePrompt ? null : localStorage.getItem(this.authStorageKey);
    if (credentials) {
      return credentials;
    }

    const username = window.prompt(`${this.authPromptMessage}\n账号:`);
    if (!username) {
      return null;
    }

    const password = window.prompt('密码:');
    if (!password) {
      return null;
    }

    credentials = btoa(`${username}:${password}`);
    localStorage.setItem(this.authStorageKey, credentials);
    return credentials;
  }

  clearStoredAuth() {
    localStorage.removeItem(this.authStorageKey);
  }
}

// 导出实例
const api = new NominatimAPI();
