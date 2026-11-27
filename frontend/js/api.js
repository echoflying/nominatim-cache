/**
 * API 客户端封装
 */

class NominatimAPI {
  constructor(baseUrl = '', useOpsSession = false) {
    this.baseUrl = baseUrl;
    this.useOpsSession = useOpsSession;
    this.credentials = null;
    this.authPromptMessage = '请输入管理后台账号和密码';
    this.lastCredentialSource = 'none';
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

  async listBatchCredentials() {
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/batch-credentials`);
    const result = await response.json();
    if (!result.success) throw new Error(result.error || '读取调用方凭据失败');
    return result.data;
  }

  async createBatchCredential(data) {
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/batch-credentials`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data)
    });
    const result = await response.json();
    if (!result.success) throw new Error(result.error || '创建调用方凭据失败');
    return result.data;
  }

  async revokeBatchCredential(id) {
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/batch-credentials/${encodeURIComponent(id)}`, { method: 'DELETE' });
    const result = await response.json();
    if (!result.success) throw new Error(result.error || '撤销调用方凭据失败');
    return result.data;
  }

  async revealBatchCredential(id) {
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/batch-credentials/${encodeURIComponent(id)}/reveal`, { method: 'POST' });
    const result = await response.json();
    if (!result.success) throw new Error(result.error || '查看调用方密钥失败');
    return result.data.token;
  }

  /**
   * 获取最近访问日志
   */
  async getRecentLogs(params = {}) {
    const searchParams = new URLSearchParams();
    if (params.page) searchParams.set('page', params.page.toString());
    if (params.limit) searchParams.set('limit', params.limit.toString());
    if (params.cached !== undefined) searchParams.set('cached', params.cached.toString());

    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/logs?${searchParams}`);
    const result = await response.json();
    if (!result.success) {
      throw new Error(result.error || 'Failed to get recent logs');
    }
    return result;
  }

  async getLogDetail(id) {
    const response = await this.authenticatedFetch(`${this.baseUrl}/admin/logs/${encodeURIComponent(id)}`);
    const result = await response.json();
    if (!result.success) {
      throw new Error(response.status === 404 ? '这条请求已过期或不存在' : result.error || '读取请求详情失败');
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
     if (params.sort) searchParams.set('sort', params.sort);
     if (params.order) searchParams.set('order', params.order);

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
    if (this.useOpsSession) {
      return fetch(url, options);
    }
    const authHeaders = this.getAuthHeaders();
    const credentialSource = this.lastCredentialSource;
    const response = await fetch(url, {
      ...options,
      headers: {
        ...authHeaders,
        ...(options.headers || {})
      }
    });

    if (response.status === 401 && allowRetry && credentialSource === 'stored') {
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
    let credentials = forcePrompt ? null : this.credentials;
    if (credentials) {
      this.lastCredentialSource = 'stored';
      return credentials;
    }

    const username = window.prompt(`${this.authPromptMessage}\n账号:`);
    if (!username) {
      this.lastCredentialSource = 'none';
      return null;
    }

    const password = window.prompt('密码:');
    if (!password) {
      this.lastCredentialSource = 'none';
      return null;
    }

    credentials = btoa(`${username}:${password}`);
    this.credentials = credentials;
    this.lastCredentialSource = 'prompt';
    return credentials;
  }

  clearStoredAuth() {
    this.credentials = null;
  }
}

// 导出实例
try {
  globalThis.localStorage?.removeItem('nominatim_auth');
} catch {
  // 浏览器禁用存储时仍可使用当前页面。
}
const baseUrl = globalThis.location ? new URL('./', globalThis.location.href).href.replace(/\/$/, '') : '';
const useOpsSession = globalThis.location?.pathname.startsWith('/ops/nominatim/') ?? false;
const api = new NominatimAPI(baseUrl, useOpsSession);
